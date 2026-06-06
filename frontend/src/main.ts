import "./style.css";

import {
  getCurrentModel,
  listModels,
  listTools,
  resolveToolPermission,
  selectModel,
  streamChat,
} from "./api";
import type { AgentEvent, ModelInfo } from "./types";
import {
  appendAgentBubble,
  appendUserBubble,
  applyEvent,
  clearMessages,
  enableTool,
  getDisabledTools,
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
const connectionDotEl = document.querySelector(".status-dot") as HTMLSpanElement;
const connectionLabelEl = document.getElementById("connectionLabel") as HTMLSpanElement;

const eventCountEl = document.getElementById("eventCount") as HTMLDivElement;
const toolCallCountEl = document.getElementById("toolCallCount") as HTMLDivElement;
const chunkCountEl = document.getElementById("chunkCount") as HTMLDivElement;

const toolListEl = document.getElementById("toolList") as HTMLDivElement;

const modelSelectEl = document.getElementById("modelSelect") as HTMLSelectElement;
const sessionInputEl = document.getElementById("sessionInput") as HTMLInputElement;
const btnNewSessionEl = document.getElementById("btnNewSession") as HTMLButtonElement;

// 主题 / 折叠相关元素 —— 新 UI 引入
const layoutEl = document.getElementById("app") as HTMLDivElement;
const btnThemeEl = document.getElementById("btnTheme") as HTMLButtonElement | null;
const themeIconEl = document.getElementById("themeIcon") as HTMLSpanElement | null;
const btnSidebarToggleEl = document.getElementById("btnSidebarToggle") as HTMLButtonElement | null;
const btnSidebarOpenEl = document.getElementById("btnSidebarOpen") as HTMLButtonElement | null;
const btnRightToggleEl = document.getElementById("btnRightToggle") as HTMLButtonElement | null;

const SESSION_STORAGE_KEY = "gora.sessionId";
const THEME_STORAGE_KEY = "gora.theme";
const SIDEBAR_COLLAPSED_KEY = "gora.sidebarCollapsed";
const RIGHT_COLLAPSED_KEY = "gora.rightCollapsed";

let inFlight: AbortController | null = null;
let availableModels: ModelInfo[] = [];
/** 当前会话 id；为空时不发送 session_id 字段，由后端使用 default。 */
let currentSessionID = "";
/**
 * 当前模型选择器（即提交给后端 /api/models/select 的 selector）。
 * 优先使用 ModelInfo.name；为空时退化到 ModelInfo.model。
 */
let currentModelSelector = "";

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
    <div class="agent-text">
      <p>你好！我是 <strong>Gora Agent</strong>。</p>
    </div>
  `;
}

function setProcessing(processing: boolean): void {
  btnSendEl.disabled = processing;
  userInputEl.disabled = processing;
  // 会话/model 切换在请求进行中也应锁定，避免与流冲突。
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

/**
 * 模型在前端的稳定 selector / 展示文案。
 *
 * 项目约定：config 里的 LLMConfig.Name 是模型唯一标识，所有展示 / 选择
 * 都用它；ModelInfo.model 仅供调用底层 LLM API 时使用，前端不在任何
 * 路径上依赖它。
 */
function modelKey(m: ModelInfo): string {
  return (m.name ?? "").trim();
}

function modelLabel(m: ModelInfo): string {
  return (m.name ?? "").trim();
}

/** 当前模型的 ModelInfo（用于头部显示 / 气泡 badge）。 */
function currentModelInfo(): ModelInfo | undefined {
  if (!currentModelSelector) return undefined;
  return availableModels.find((m) => modelKey(m) === currentModelSelector);
}

function currentModelDisplay(): string {
  const m = currentModelInfo();
  return m ? modelLabel(m) : "";
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
    option.value = modelKey(m);
    option.textContent = modelLabel(m);
    if (option.value === currentModelSelector) option.selected = true;
    modelSelectEl.appendChild(option);
  }
}

function syncModelOnHeader(): void {
  // 模型展示已合并进底部 select，这里只负责处理"无模型可选"时的占位。
  if (availableModels.length === 0 && modelSelectEl.options.length === 0) {
    const option = document.createElement("option");
    option.value = "";
    option.textContent = "（不可切换）";
    modelSelectEl.appendChild(option);
  }
}

async function refreshCurrentModel(): Promise<void> {
  if (availableModels.length === 0) return;
  try {
    const res = await getCurrentModel(currentSessionID);
    if (res) {
      currentModelSelector = modelKey(res.model);
      renderModelSelect();
    }
  } catch (err) {
    console.warn("getCurrentModel 失败", err);
  }
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
  // 先恢复持久化的 session，再拉接口。
  currentSessionID = loadStoredString(SESSION_STORAGE_KEY);
  sessionInputEl.value = currentSessionID;

  pushWelcome();
  renderConnection(connectionDotEl, connectionLabelEl, "connecting", "连接中…");

  try {
    const [tools, models] = await Promise.all([
      listTools(),
      listModels(),
    ]);
    availableModels = models;

    // 模型：先渲染列表，再异步拉取当前 session 的选择。
    renderModelSelect();
    if (availableModels.length > 0) {
      await refreshCurrentModel();
    } else {
      // 无可选模型时，给 select 放一个占位 option。
      syncModelOnHeader();
      setAgentState(agentStateEl, "idle");
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
  const nextSelector = modelSelectEl.value.trim();
  if (!nextSelector || nextSelector === currentModelSelector) return;

  const previousSelector = currentModelSelector;
  modelSelectEl.disabled = true;
  try {
    const next = await selectModel(currentSessionID, nextSelector);
    currentModelSelector = modelKey(next);
    syncModelOnHeader();
  } catch (err) {
    console.error("切换模型失败", err);
    // 还原选择
    currentModelSelector = previousSelector;
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

  let handle = appendAgentBubble(messagesEl, handleToolPermission, currentModelDisplay());
  inFlight = new AbortController();

  try {
    const disabledTools = [...getDisabledTools()];
    await streamChat(
      {
        message: trimmed,
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
  userInputEl.style.height = `${Math.min(userInputEl.scrollHeight, 200)}px`;
}

/* ============================================================
 * 主题切换：light / dark；默认跟随系统 prefers-color-scheme。
 * ========================================================== */
type Theme = "light" | "dark";

function detectInitialTheme(): Theme {
  const stored = loadStoredString(THEME_STORAGE_KEY);
  if (stored === "light" || stored === "dark") return stored;
  const prefersDark =
    typeof window.matchMedia === "function" &&
    window.matchMedia("(prefers-color-scheme: dark)").matches;
  return prefersDark ? "dark" : "light";
}

function applyTheme(theme: Theme, persist = true): void {
  document.documentElement.setAttribute("data-theme", theme);
  if (themeIconEl) {
    // 太阳代表"切到亮色"；月亮代表"切到暗色"。
    themeIconEl.textContent = theme === "dark" ? "☀️" : "🌙";
  }
  if (btnThemeEl) {
    const next = theme === "dark" ? "亮色" : "暗色";
    btnThemeEl.title = `切换到${next}主题`;
    btnThemeEl.setAttribute("aria-label", btnThemeEl.title);
  }
  if (persist) saveStoredString(THEME_STORAGE_KEY, theme);
}

function toggleTheme(): void {
  const cur = (document.documentElement.getAttribute("data-theme") as Theme) || "light";
  applyTheme(cur === "dark" ? "light" : "dark");
}

/* ============================================================
 * 折叠 sidebar / 右侧面板。状态写到 localStorage。
 * ========================================================== */
function applySidebarState(collapsed: boolean, persist = true): void {
  layoutEl.classList.toggle("sidebar-collapsed", collapsed);
  if (persist) saveStoredString(SIDEBAR_COLLAPSED_KEY, collapsed ? "1" : "");
}

function applyRightState(collapsed: boolean, persist = true): void {
  layoutEl.classList.toggle("right-collapsed", collapsed);
  if (btnRightToggleEl) {
    btnRightToggleEl.setAttribute("aria-expanded", collapsed ? "false" : "true");
  }
  if (persist) saveStoredString(RIGHT_COLLAPSED_KEY, collapsed ? "1" : "");
}

/* ============================================================
 * 事件绑定
 * ========================================================== */
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

// 示例 prompt 卡片（新 UI 改名为 .example-card）
document.querySelectorAll<HTMLButtonElement>(".example-card").forEach((el) => {
  el.addEventListener("click", () => {
    const prompt = el.dataset["prompt"];
    if (prompt) void sendMessage(prompt);
  });
});

// 主题切换
btnThemeEl?.addEventListener("click", () => toggleTheme());

// sidebar 折叠 / 展开
btnSidebarToggleEl?.addEventListener("click", () => applySidebarState(true));
btnSidebarOpenEl?.addEventListener("click", () => applySidebarState(false));

// 右侧面板折叠（topbar 上的按钮在折叠/展开间切换）
btnRightToggleEl?.addEventListener("click", () => {
  const collapsed = layoutEl.classList.contains("right-collapsed");
  applyRightState(!collapsed);
});

window.addEventListener("beforeunload", () => {
  inFlight?.abort();
});

/* ============================================================
 * 启动
 * ========================================================== */
applyTheme(detectInitialTheme(), false);
applySidebarState(loadStoredString(SIDEBAR_COLLAPSED_KEY) === "1", false);
applyRightState(loadStoredString(RIGHT_COLLAPSED_KEY) === "1", false);

void bootstrap();
wireCollapsibleSections();
userInputEl.focus();
