import type { AgentEvent, AgentInfo, ChatRequest, ToolInfo } from "./types";

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
