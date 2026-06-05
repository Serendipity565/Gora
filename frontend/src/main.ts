import "./style.css";

import {
  getCurrentModel,
  listAgents,
  listModels,
  listTools,
  resolveToolPermission,
  selectModel,
  streamChat,
} from "./api";
import type { AgentEvent, AgentInfo, ModelInfo } from "./types";
import {
  appendAgentBubble,
  appendUserBubble,
  applyEvent,
  clearMessages,
  enableTool,
  getDisabledTools,
  renderAgentMeta,
  renderAgentOptions,
  renderConnection,
  renderToolList,
  scrollToBottom,
  setAgentState,
  wireCollapsibleSections,
} from "./ui";
import type { PermissionDecision } from "./ui";

const messagesEl = document.getElementById("messages") as HTMLDivElement;
const composerEl = document.getElementById("composer") as HTMLFormElement;
const userInputEl = document.getElementById("userInput") as HTMLTextAreaElement;
const btnSendEl = document.getElementById("btnSend") as HTMLButtonElement;

const agentStateEl = document.getElementById("agentState") as HTMLSpanElement;
const modelNameEl = document.getElementById("modelName") as HTMLSpanElement;
const connectionDotEl = document.querySelector(".status-dot") as HTMLSpanElement;
const connectionLabelEl = document.getElementById("connectionLabel") as HTMLSpanElement;

const eventCountEl = document.getElementById("eventCount") as HTMLDivElement;
const toolCallCountEl = document.getElementById("toolCallCount") as HTMLDivElement;
const chunkCountEl = document.getElementById("chunkCount") as HTMLDivElement;

const toolListEl = document.getElementById("toolList") as HTMLDivElement;

const agentSelectEl = document.getElementById("agentSelect") as HTMLSelectElement;
const modelSelectEl = document.getElementById("modelSelect") as HTMLSelectElement;
const sessionInputEl = document.getElementById("sessionInput") as HTMLInputElement;
const btnNewSessionEl = document.getElementById("btnNewSession") as HTMLButtonElement;

const SESSION_STORAGE_KEY = "gora.sessionId";
const AGENT_STORAGE_KEY = "gora.agentId";

let inFlight: AbortController | null = null;
let knownAgents: AgentInfo[] = [];
let availableModels: ModelInfo[] = [];
/** 当前选中的 agent；空串表示让后端选默认。 */
let currentAgentID = "";
/** 当前会话 id；为空时不发送 session_id 字段，由后端使用 default。 */
let currentSessionID = "";
/** 当前模型 index；-1 表示尚未确定（接口失败 / 未启用）。 */
let currentModelIndex = -1;

function loadStoredString(key: string): string {
  try {
    const raw = window.localStorage.getItem(key);
    return raw && raw.trim() ? raw.trim() : "";
  } catch {
    return "";
  }
}

function saveStoredString(key: string, value: string): void {
  try {
    if (value) window.localStorage.setItem(key, value);
    else window.localStorage.removeItem(key);
  } catch {
    // localStorage 不可用时忽略
  }
}

function generateSessionId(): string {
  const cryptoObj = (window.crypto || (window as unknown as { msCrypto?: Crypto }).msCrypto) as Crypto | undefined;
  if (cryptoObj?.randomUUID) {
    return `session-${cryptoObj.randomUUID().slice(0, 8)}`;
  }
  return `session-${Date.now().toString(36)}`;
}

function pushWelcome(): void {
  const handle = appendAgentBubble(messagesEl);
  handle.bubble.innerHTML = `
    你好！我是 <strong>Gora Agent</strong>。<br />
    我可以调用工具来完成任务。<br />
    试试问我：「<em>帮我查一下 https://api.github.com 的状态</em>」
  `;
}

function setProcessing(processing: boolean): void {
  btnSendEl.disabled = processing;
  userInputEl.disabled = processing;
  // 会话/agent/model 切换在请求进行中也应锁定，避免与流冲突。
  agentSelectEl.disabled = processing || agentSelectEl.options.length === 0;
  modelSelectEl.disabled = processing || availableModels.length === 0;
  sessionInputEl.disabled = processing;
  btnNewSessionEl.disabled = processing;
}

function resetCounters(): { event: number; tool: number; chunk: number } {
  const counters = { event: 0, tool: 0, chunk: 0 };
  eventCountEl.textContent = "0";
  toolCallCountEl.textContent = "0";
  chunkCountEl.textContent = "0";
  return counters;
}

function updateCounters(counters: { event: number; tool: number; chunk: number }): void {
  eventCountEl.textContent = String(counters.event);
  toolCallCountEl.textContent = String(counters.tool);
  chunkCountEl.textContent = String(counters.chunk);
}

function renderModelSelect(): void {
  modelSelectEl.innerHTML = "";
  if (availableModels.length === 0) {
    const option = document.createElement("option");
    option.value = "";
    option.textContent = "（不可切换）";
    modelSelectEl.appendChild(option);
    modelSelectEl.disabled = true;
    return;
  }
  modelSelectEl.disabled = false;
  for (const m of availableModels) {
    const option = document.createElement("option");
    option.value = String(m.index);
    option.textContent = m.display;
    if (m.index === currentModelIndex) option.selected = true;
    modelSelectEl.appendChild(option);
  }
}

function syncModelOnHeader(): void {
  const target = availableModels.find((m) => m.index === currentModelIndex);
  if (target) {
    modelNameEl.textContent = `model: ${target.display}`;
  }
}

async function refreshCurrentModel(): Promise<void> {
  if (availableModels.length === 0) return;
  try {
    const res = await getCurrentModel(currentSessionID);
    if (res) {
      currentModelIndex = res.model.index;
      renderModelSelect();
      syncModelOnHeader();
    }
  } catch (err) {
    console.warn("getCurrentModel 失败", err);
  }
}

function syncAgentMeta(): void {
  const target = knownAgents.find((a) => a.id === currentAgentID) ?? knownAgents[0];
  if (!target) {
    renderAgentMeta(modelNameEl, agentStateEl, []);
    return;
  }
  renderAgentMeta(modelNameEl, agentStateEl, [target]);
  // syncAgentMeta 后再用模型信息覆盖一次，避免 agent 元数据上的旧 model 字段把它盖掉。
  syncModelOnHeader();
}

function applyAgentSelection(nextID: string): void {
  currentAgentID = nextID;
  saveStoredString(AGENT_STORAGE_KEY, currentAgentID);
  syncAgentMeta();
}

function applySessionInput(): void {
  const trimmed = sessionInputEl.value.trim();
  currentSessionID = trimmed;
  sessionInputEl.value = trimmed;
  saveStoredString(SESSION_STORAGE_KEY, currentSessionID);
}

function startNewSession(): void {
  if (inFlight) return; // 请求中不允许切会话
  const next = generateSessionId();
  sessionInputEl.value = next;
  applySessionInput();
  clearMessages(messagesEl);
  resetCounters();
  pushWelcome();
  setAgentState(agentStateEl, "idle");
}

async function bootstrap(): Promise<void> {
  // 先恢复持久化的 agent / session，再拉接口。
  currentAgentID = loadStoredString(AGENT_STORAGE_KEY);
  currentSessionID = loadStoredString(SESSION_STORAGE_KEY);
  sessionInputEl.value = currentSessionID;

  pushWelcome();
  renderConnection(connectionDotEl, connectionLabelEl, "connecting", "连接中…");

  try {
    const [agents, tools, models] = await Promise.all([
      listAgents(),
      listTools(),
      listModels(),
    ]);
    knownAgents = agents;
    availableModels = models;

    const resolved = renderAgentOptions(agentSelectEl, agents, currentAgentID);
    if (resolved !== currentAgentID) {
      applyAgentSelection(resolved);
    } else {
      syncAgentMeta();
    }

    // 模型：先渲染列表，再异步拉取当前 session 的选择。
    renderModelSelect();
    if (availableModels.length > 0) {
      await refreshCurrentModel();
    }

    renderToolList(toolListEl, tools);
    renderConnection(connectionDotEl, connectionLabelEl, "online", "在线");
  } catch (err) {
    console.error(err);
    renderConnection(
      connectionDotEl,
      connectionLabelEl,
      "error",
      "无法连接后端 (默认 :8080)",
    );
    renderToolList(toolListEl, []);
  }
}

async function handleModelChange(): Promise<void> {
  if (inFlight) return;
  const selectorRaw = modelSelectEl.value;
  if (!selectorRaw) return;
  const indexNum = Number(selectorRaw);
  if (!Number.isFinite(indexNum) || indexNum === currentModelIndex) return;

  const previousIndex = currentModelIndex;
  modelSelectEl.disabled = true;
  try {
    const next = await selectModel(currentSessionID, String(indexNum));
    currentModelIndex = next.index;
    syncModelOnHeader();
  } catch (err) {
    console.error("切换模型失败", err);
    // 还原选择
    currentModelIndex = previousIndex;
    renderModelSelect();
    alert(`切换模型失败：${(err as Error)?.message ?? String(err)}`);
  } finally {
    modelSelectEl.disabled = inFlight !== null || availableModels.length === 0;
  }
}

async function handleToolPermission(decision: PermissionDecision): Promise<void> {
  await resolveToolPermission(decision.requestID, decision.approve, decision.remember);
  if (decision.approve && decision.remember) {
    // "允许并启用" —— 把工具从前端禁用集合中移除，同步开关视觉状态。
    enableTool(decision.toolName, toolListEl);
  }
}

async function sendMessage(message: string): Promise<void> {
  const trimmed = message.trim();
  if (!trimmed || inFlight) return;

  appendUserBubble(messagesEl, trimmed);
  userInputEl.value = "";
  autoResize();

  const counters = resetCounters();
  setProcessing(true);
  setAgentState(agentStateEl, "running");

  let handle = appendAgentBubble(messagesEl, handleToolPermission);
  inFlight = new AbortController();

  try {
    const disabledTools = [...getDisabledTools()];
    await streamChat(
      {
        message: trimmed,
        ...(currentAgentID ? { agent_id: currentAgentID } : {}),
        ...(currentSessionID ? { session_id: currentSessionID } : {}),
        ...(disabledTools.length ? { disabled_tools: disabledTools } : {}),
      },
      (event: AgentEvent) => {
        counters.event += 1;
        handle = applyEvent(handle, event, counters);
        updateCounters(counters);

        if (event.type === "tool_call") {
          setAgentState(agentStateEl, "running");
        } else if (event.type === "thinking") {
          setAgentState(agentStateEl, "waiting");
        } else if (event.type === "error") {
          setAgentState(agentStateEl, "error");
        } else if (event.type === "done") {
          setAgentState(agentStateEl, "done");
        }
        scrollToBottom(messagesEl);
      },
      inFlight.signal,
    );
  } catch (err) {
    if ((err as Error)?.name === "AbortError") {
      // 主动取消，不渲染错误事件。
    } else {
      console.error(err);
      handle = applyEvent(handle, {
        type: "error",
        content: (err as Error)?.message ?? String(err),
      }, counters);
      setAgentState(agentStateEl, "error");
    }
  } finally {
    inFlight = null;
    setProcessing(false);
    if (agentStateEl.textContent !== "错误") {
      setAgentState(agentStateEl, "idle");
    }
    userInputEl.focus();
  }
}

function autoResize(): void {
  userInputEl.style.height = "auto";
  userInputEl.style.height = `${Math.min(userInputEl.scrollHeight, 160)}px`;
}

composerEl.addEventListener("submit", (event) => {
  event.preventDefault();
  void sendMessage(userInputEl.value);
});

userInputEl.addEventListener("keydown", (event) => {
  if (event.key === "Enter" && !event.shiftKey && !event.isComposing) {
    event.preventDefault();
    void sendMessage(userInputEl.value);
  }
});

userInputEl.addEventListener("input", autoResize);

agentSelectEl.addEventListener("change", () => {
  applyAgentSelection(agentSelectEl.value);
});

modelSelectEl.addEventListener("change", () => {
  void handleModelChange();
});

sessionInputEl.addEventListener("change", () => {
  applySessionInput();
  void refreshCurrentModel();
});
sessionInputEl.addEventListener("blur", () => {
  applySessionInput();
  void refreshCurrentModel();
});

btnNewSessionEl.addEventListener("click", () => {
  startNewSession();
  void refreshCurrentModel();
});

document.querySelectorAll<HTMLButtonElement>(".tool-item.example").forEach((el) => {
  el.addEventListener("click", () => {
    const prompt = el.dataset["prompt"];
    if (prompt) void sendMessage(prompt);
  });
});

window.addEventListener("beforeunload", () => {
  inFlight?.abort();
});

void bootstrap();
wireCollapsibleSections();
userInputEl.focus();
