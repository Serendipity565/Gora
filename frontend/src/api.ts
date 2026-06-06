import type { AgentEvent, AgentInfo, ChatRequest, ModelInfo, ToolInfo } from "./types";

// dev 环境通过 vite 代理把 /api 转发到 :8080；
// 生产环境后端会直接 host 静态文件，路径相对即可。
const API_BASE = "";

export async function listAgents(): Promise<AgentInfo[]> {
  const res = await fetch(`${API_BASE}/api/agents`);
  if (!res.ok) {
    throw new Error(`GET /api/agents 失败: ${res.status}`);
  }
  const data = (await res.json()) as { agents: AgentInfo[] };
  return data.agents ?? [];
}

export async function listTools(): Promise<ToolInfo[]> {
  const res = await fetch(`${API_BASE}/api/tools`);
  if (!res.ok) {
    throw new Error(`GET /api/tools 失败: ${res.status}`);
  }
  const data = (await res.json()) as { tools: ToolInfo[] };
  return data.tools ?? [];
}

/**
 * 列出所有可选模型；后端未配置 ModelSelector 时返回 501，
 * 这里转换成空列表，调用方可据此隐藏模型 UI。
 */
export async function listModels(): Promise<ModelInfo[]> {
  const res = await fetch(`${API_BASE}/api/models`);
  if (res.status === 501) return [];
  if (!res.ok) {
    throw new Error(`GET /api/models 失败: ${res.status}`);
  }
  const data = (await res.json()) as { models: ModelInfo[] };
  return data.models ?? [];
}

/** 查询某 session 当前使用的模型；无显式选择时返回默认模型。 */
export async function getCurrentModel(
  sessionID: string,
): Promise<{ model: ModelInfo; explicit: boolean } | null> {
  const url = sessionID
    ? `${API_BASE}/api/models/current?session_id=${encodeURIComponent(sessionID)}`
    : `${API_BASE}/api/models/current`;
  const res = await fetch(url);
  if (res.status === 501) return null;
  if (!res.ok) {
    throw new Error(`GET /api/models/current 失败: ${res.status}`);
  }
  return (await res.json()) as { model: ModelInfo; explicit: boolean };
}

/**
 * 把某 session 切换到 selector 指定的模型。
 *
 * selector 取值固定为 LLMConfig.Name —— 项目约定 name 是模型唯一标识。
 * 字段名保留 selector 仅是 API 兼容；语义上等价于 model name。
 */
export async function selectModel(sessionID: string, selector: string): Promise<ModelInfo> {
  const res = await fetch(`${API_BASE}/api/models/select`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      session_id: sessionID,
      selector,
    }),
  });
  if (!res.ok) {
    let detail = "";
    try {
      detail = (await res.text()).slice(0, 200);
    } catch {
      // ignore
    }
    throw new Error(`POST /api/models/select 失败: ${res.status} ${detail}`);
  }
  const data = (await res.json()) as { model: ModelInfo };
  return data.model;
}

/**
 * 把"是否允许调用某个被禁用的工具"的决定回写给后端。
 * 后端会唤醒等待中的 Agent goroutine，继续/放弃该工具调用。
 */
export async function resolveToolPermission(
  requestID: string,
  approve: boolean,
  remember = false,
): Promise<void> {
  const res = await fetch(`${API_BASE}/api/chat/tool-permission`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      request_id: requestID,
      approve,
      remember,
    }),
  });
  if (!res.ok) {
    let detail = "";
    try {
      detail = (await res.text()).slice(0, 200);
    } catch {
      // ignore
    }
    throw new Error(`POST /api/chat/tool-permission 失败: ${res.status} ${detail}`);
  }
}

/**
 * 通过 fetch 读取 SSE 流，逐事件回调。返回的 Promise 在流结束时 resolve。
 * 注意：浏览器 EventSource 不支持 POST，这里手动解析 text/event-stream。
 */
export async function streamChat(
  request: ChatRequest,
  onEvent: (event: AgentEvent) => void,
  signal?: AbortSignal,
): Promise<void> {
  const res = await fetch(`${API_BASE}/api/chat`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Accept: "text/event-stream",
    },
    body: JSON.stringify(request),
    signal,
  });

  if (!res.ok || !res.body) {
    let detail = "";
    try {
      detail = (await res.text()).slice(0, 200);
    } catch {
      // ignore
    }
    throw new Error(`POST /api/chat 失败: ${res.status} ${detail}`);
  }

  const reader = res.body.getReader();
  const decoder = new TextDecoder("utf-8");
  let buffer = "";

  // SSE 帧用空行分隔，每帧可能包含多行 event:/data: 字段。
  while (true) {
    const { value, done } = await reader.read();
    if (done) break;

    buffer += decoder.decode(value, { stream: true });

    let separator: number;
    while ((separator = buffer.indexOf("\n\n")) !== -1) {
      const frame = buffer.slice(0, separator);
      buffer = buffer.slice(separator + 2);
      const event = parseSSEFrame(frame);
      if (event) onEvent(event);
    }
  }

  // 处理流末尾不带空行的残余数据。
  if (buffer.trim().length > 0) {
    const event = parseSSEFrame(buffer);
    if (event) onEvent(event);
  }
}

function parseSSEFrame(frame: string): AgentEvent | null {
  let dataPayload = "";
  let eventType: string | null = null;

  for (const rawLine of frame.split("\n")) {
    const line = rawLine.replace(/\r$/, "");
    if (line.startsWith(":")) continue; // SSE 注释
    if (line.startsWith("event:")) {
      eventType = line.slice(6).trim();
    } else if (line.startsWith("data:")) {
      // 多行 data 字段按规范用 \n 拼接
      dataPayload += (dataPayload ? "\n" : "") + line.slice(5).trim();
    }
  }

  if (!dataPayload) return null;

  try {
    const parsed = JSON.parse(dataPayload) as Partial<AgentEvent>;
    return {
      ...parsed,
      type: (parsed.type ?? eventType ?? "unknown") as AgentEvent["type"],
    } as AgentEvent;
  } catch {
    // 兜底：把无法解析的 data 包成 chunk 事件，避免静默丢失。
    return {
      type: (eventType as AgentEvent["type"]) ?? "unknown",
      content: dataPayload,
    };
  }
}
