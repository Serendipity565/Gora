import "./style.css";

import {
  getCurrentModel,
  listModels,
  listTools,
  resolveToolPermission,
  selectModel,
  streamChat,
} from "./api";
import { clearSession, getUser, isLoggedIn, onAuthChange, setSession } from "./auth";
import type { AuthUser } from "./auth";
import { login as apiLogin, register as apiRegister } from "./userApi";
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
  scrollToBottomIfPinned,
  setAgentState,
  wireCollapsibleSections,
} from "./ui";
import type { PermissionDecision } from "./ui";

/* ============================================================
 * 视图切换：authView (登录/注册)  ↔  chatView (主聊天界面)
 * ========================================================== */
const authViewEl = document.getElementById("authView") as HTMLElement;
const chatViewEl = document.getElementById("chatView") as HTMLElement;

function showAuthView(): void {
  authViewEl.hidden = false;
  chatViewEl.hidden = true;
}

function showChatView(): void {
  authViewEl.hidden = true;
  chatViewEl.hidden = false;
}

/* ============================================================
 * Chat 视图相关元素
 * ========================================================== */
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

const btnThemeEl = document.getElementById("btnTheme") as HTMLButtonElement | null;
const themeIconEl = document.getElementById("themeIcon") as HTMLSpanElement | null;
const btnSidebarToggleEl = document.getElementById("btnSidebarToggle") as HTMLButtonElement | null;
const btnSidebarOpenEl = document.getElementById("btnSidebarOpen") as HTMLButtonElement | null;
const btnRightToggleEl = document.getElementById("btnRightToggle") as HTMLButtonElement | null;

// 用户菜单
const btnUserEl = document.getElementById("btnUser") as HTMLButtonElement | null;
const userAvatarEl = document.getElementById("userAvatar") as HTMLSpanElement | null;
const userNameEl = document.getElementById("userName") as HTMLSpanElement | null;
const userMenuPopoverEl = document.getElementById("userMenuPopover") as HTMLDivElement | null;
const userMenuNameEl = document.getElementById("userMenuName") as HTMLDivElement | null;
const userMenuEmailEl = document.getElementById("userMenuEmail") as HTMLDivElement | null;
const btnLogoutEl = document.getElementById("btnLogout") as HTMLButtonElement | null;

/* ============================================================
 * Auth 视图相关元素
 * ========================================================== */
const tabLoginEl = document.getElementById("tabLogin") as HTMLButtonElement;
const tabRegisterEl = document.getElementById("tabRegister") as HTMLButtonElement;
const loginFormEl = document.getElementById("loginForm") as HTMLFormElement;
const registerFormEl = document.getElementById("registerForm") as HTMLFormElement;
const loginEmailEl = document.getElementById("loginEmail") as HTMLInputElement;
const loginPasswordEl = document.getElementById("loginPassword") as HTMLInputElement;
const btnLoginEl = document.getElementById("btnLogin") as HTMLButtonElement;
const loginErrorEl = document.getElementById("loginError") as HTMLParagraphElement;
const registerEmailEl = document.getElementById("registerEmail") as HTMLInputElement;
const registerUsernameEl = document.getElementById("registerUsername") as HTMLInputElement;
const registerPasswordEl = document.getElementById("registerPassword") as HTMLInputElement;
const btnRegisterEl = document.getElementById("btnRegister") as HTMLButtonElement;
const registerErrorEl = document.getElementById("registerError") as HTMLParagraphElement;
const btnAuthThemeEl = document.getElementById("btnAuthTheme") as HTMLButtonElement | null;
const authThemeIconEl = document.getElementById("authThemeIcon") as HTMLSpanElement | null;

/* ============================================================
 * 持久化 key
 * ========================================================== */
const SESSION_STORAGE_KEY = "gora.sessionId";
const THEME_STORAGE_KEY = "gora.theme";
const SIDEBAR_COLLAPSED_KEY = "gora.sidebarCollapsed";
const RIGHT_COLLAPSED_KEY = "gora.rightCollapsed";

/* ============================================================
 * 状态
 * ========================================================== */
let inFlight: AbortController | null = null;
let availableModels: ModelInfo[] = [];
let currentSessionID = "";
let currentModelSelector = "";
let chatBootstrapped = false;

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
  const user = getUser();
  const greeting = user?.username ? `你好，${escapeHTML(user.username)}！` : "你好！";
  handle.bubble.innerHTML = `
    <div class="agent-text">
      <p>${greeting}我是 <strong>Gora Agent</strong>。</p>
    </div>
  `;
}

function escapeHTML(text: string): string {
  return text
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;");
}

function setProcessing(processing: boolean): void {
  btnSendEl.disabled = processing;
  userInputEl.disabled = processing;
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

function modelKey(m: ModelInfo): string {
  return (m.name ?? "").trim();
}

function modelLabel(m: ModelInfo): string {
  return (m.name ?? "").trim();
}

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
  if (inFlight) return;
  const next = generateSessionId();
  sessionInputEl.value = next;
  applySessionInput();
  clearMessages(messagesEl);
  resetCounters();
  pushWelcome();
  setAgentState(agentStateEl, "idle");
}

/* ============================================================
 * 用户菜单
 * ========================================================== */
function applyUserToMenu(user: AuthUser | null): void {
  if (!user) {
    if (userAvatarEl) userAvatarEl.textContent = "G";
    if (userNameEl) userNameEl.textContent = "未登录";
    if (userMenuNameEl) userMenuNameEl.textContent = "—";
    if (userMenuEmailEl) userMenuEmailEl.textContent = "—";
    return;
  }
  const initial = (user.username || user.email || "G").trim().charAt(0).toUpperCase();
  if (userAvatarEl) userAvatarEl.textContent = initial || "G";
  if (userNameEl) userNameEl.textContent = user.username || user.email;
  if (userMenuNameEl) userMenuNameEl.textContent = user.username || "(无昵称)";
  if (userMenuEmailEl) userMenuEmailEl.textContent = user.email;
}

function toggleUserMenu(force?: boolean): void {
  if (!userMenuPopoverEl || !btnUserEl) return;
  const next = typeof force === "boolean" ? force : userMenuPopoverEl.hidden;
  userMenuPopoverEl.hidden = !next;
  btnUserEl.setAttribute("aria-expanded", next ? "true" : "false");
}

document.addEventListener("click", (e) => {
  if (!userMenuPopoverEl || userMenuPopoverEl.hidden) return;
  const target = e.target as Node;
  if (btnUserEl?.contains(target) || userMenuPopoverEl.contains(target)) return;
  toggleUserMenu(false);
});

btnUserEl?.addEventListener("click", () => toggleUserMenu());

btnLogoutEl?.addEventListener("click", () => {
  toggleUserMenu(false);
  clearSession();
});

/* ============================================================
 * Chat 视图初始化
 * ========================================================== */
async function bootstrapChat(): Promise<void> {
  if (chatBootstrapped) {
    // 重新进入（如登出后再登录）时只重置 UI 状态，不重复绑事件。
    clearMessages(messagesEl);
    resetCounters();
    pushWelcome();
    return;
  }
  chatBootstrapped = true;

  currentSessionID = loadStoredString(SESSION_STORAGE_KEY);
  sessionInputEl.value = currentSessionID;

  pushWelcome();
  renderConnection(connectionDotEl, connectionLabelEl, "connecting", "连接中…");

  try {
    const [tools, models] = await Promise.all([listTools(), listModels()]);
    availableModels = models;

    renderModelSelect();
    if (availableModels.length > 0) {
      await refreshCurrentModel();
    } else {
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
        // 仅当用户没主动往上翻历史时才贴底；
        // 否则保持当前阅读位置，避免被流式事件强制拉走。
        scrollToBottomIfPinned(messagesEl);
      },
      inFlight.signal,
    );
  } catch (err) {
    if ((err as Error)?.name === "AbortError") {
      // 主动取消
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
 * 主题切换
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
  const icon = theme === "dark" ? "☀️" : "🌙";
  if (themeIconEl) themeIconEl.textContent = icon;
  if (authThemeIconEl) authThemeIconEl.textContent = icon;
  const next = theme === "dark" ? "亮色" : "暗色";
  if (btnThemeEl) {
    btnThemeEl.title = `切换到${next}主题`;
    btnThemeEl.setAttribute("aria-label", btnThemeEl.title);
  }
  if (btnAuthThemeEl) {
    btnAuthThemeEl.title = `切换到${next}主题`;
    btnAuthThemeEl.setAttribute("aria-label", btnAuthThemeEl.title);
  }
  if (persist) saveStoredString(THEME_STORAGE_KEY, theme);
}

function toggleTheme(): void {
  const cur = (document.documentElement.getAttribute("data-theme") as Theme) || "light";
  applyTheme(cur === "dark" ? "light" : "dark");
}

/* ============================================================
 * 折叠 sidebar / 右侧面板
 * ========================================================== */
function applySidebarState(collapsed: boolean, persist = true): void {
  chatViewEl.classList.toggle("sidebar-collapsed", collapsed);
  if (persist) saveStoredString(SIDEBAR_COLLAPSED_KEY, collapsed ? "1" : "");
}

function applyRightState(collapsed: boolean, persist = true): void {
  chatViewEl.classList.toggle("right-collapsed", collapsed);
  if (btnRightToggleEl) {
    btnRightToggleEl.setAttribute("aria-expanded", collapsed ? "false" : "true");
  }
  if (persist) saveStoredString(RIGHT_COLLAPSED_KEY, collapsed ? "1" : "");
}

/* ============================================================
 * 登录 / 注册
 * ========================================================== */
function showAuthError(target: HTMLParagraphElement, msg: string): void {
  target.textContent = msg;
  target.hidden = false;
}

function clearAuthError(target: HTMLParagraphElement): void {
  target.textContent = "";
  target.hidden = true;
}

function setActiveTab(tab: "login" | "register"): void {
  const isLogin = tab === "login";
  tabLoginEl.classList.toggle("is-active", isLogin);
  tabRegisterEl.classList.toggle("is-active", !isLogin);
  tabLoginEl.setAttribute("aria-selected", isLogin ? "true" : "false");
  tabRegisterEl.setAttribute("aria-selected", isLogin ? "false" : "true");
  loginFormEl.hidden = !isLogin;
  registerFormEl.hidden = isLogin;
  clearAuthError(loginErrorEl);
  clearAuthError(registerErrorEl);
}

// EMAIL_REGEX 与后端 validator.v10 的 "email" tag 对齐到常见用法：
// 必须含 @，本地段非空，域名段至少含一个 . 且末尾为字母。
// 不追求 RFC 5322 完美兼容；后端会再校一次。
const EMAIL_REGEX = /^[^\s@]+@[^\s@]+\.[A-Za-z]{2,}$/;

function isValidEmail(value: string): boolean {
  return EMAIL_REGEX.test(value);
}

tabLoginEl.addEventListener("click", () => setActiveTab("login"));
tabRegisterEl.addEventListener("click", () => setActiveTab("register"));

loginFormEl.addEventListener("submit", async (event) => {
  event.preventDefault();
  clearAuthError(loginErrorEl);

  const email = loginEmailEl.value.trim();
  const password = loginPasswordEl.value;
  if (!email || !password) {
    showAuthError(loginErrorEl, "请填写邮箱和密码");
    return;
  }
  if (!isValidEmail(email)) {
    showAuthError(loginErrorEl, "邮箱格式不正确");
    return;
  }

  btnLoginEl.disabled = true;
  try {
    const session = await apiLogin({ email, password });
    setSession(session);
  } catch (err) {
    showAuthError(loginErrorEl, (err as Error)?.message || "登录失败");
  } finally {
    btnLoginEl.disabled = false;
  }
});

registerFormEl.addEventListener("submit", async (event) => {
  event.preventDefault();
  clearAuthError(registerErrorEl);

  const email = registerEmailEl.value.trim();
  const username = registerUsernameEl.value.trim();
  const password = registerPasswordEl.value;
  if (!email || !username || !password) {
    showAuthError(registerErrorEl, "请完整填写注册信息");
    return;
  }
  if (!isValidEmail(email)) {
    showAuthError(registerErrorEl, "邮箱格式不正确");
    return;
  }
  if (username.length < 2 || username.length > 50) {
    showAuthError(registerErrorEl, "用户名长度需在 2 ~ 50 字之间");
    return;
  }
  if (password.length < 6 || password.length > 64) {
    showAuthError(registerErrorEl, "密码长度需在 6 ~ 64 位之间");
    return;
  }

  btnRegisterEl.disabled = true;
  try {
    await apiRegister({ email, username, password });
    // 注册成功后自动用同一对邮箱密码登录，避免用户再点一次。
    const session = await apiLogin({ email, password });
    setSession(session);
  } catch (err) {
    showAuthError(registerErrorEl, (err as Error)?.message || "注册失败");
  } finally {
    btnRegisterEl.disabled = false;
  }
});

btnAuthThemeEl?.addEventListener("click", () => toggleTheme());

/* ============================================================
 * Chat 视图事件绑定
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

document.querySelectorAll<HTMLButtonElement>(".example-card").forEach((el) => {
  el.addEventListener("click", () => {
    const prompt = el.dataset["prompt"];
    if (prompt) void sendMessage(prompt);
  });
});

btnThemeEl?.addEventListener("click", () => toggleTheme());

btnSidebarToggleEl?.addEventListener("click", () => applySidebarState(true));
btnSidebarOpenEl?.addEventListener("click", () => applySidebarState(false));

btnRightToggleEl?.addEventListener("click", () => {
  const collapsed = chatViewEl.classList.contains("right-collapsed");
  applyRightState(!collapsed);
});

window.addEventListener("beforeunload", () => {
  inFlight?.abort();
});

/* ============================================================
 * Auth 状态变化：登录后切到 chat 视图，登出回到登录页
 * ========================================================== */
function applyAuthView(): void {
  if (isLoggedIn()) {
    applyUserToMenu(getUser());
    showChatView();
    void bootstrapChat();
  } else {
    inFlight?.abort();
    inFlight = null;
    applyUserToMenu(null);
    toggleUserMenu(false);
    setActiveTab("login");
    loginEmailEl.value = "";
    loginPasswordEl.value = "";
    showAuthView();
    setTimeout(() => loginEmailEl.focus(), 0);
  }
}

onAuthChange(() => applyAuthView());

/* ============================================================
 * 启动
 * ========================================================== */
applyTheme(detectInitialTheme(), false);
applySidebarState(loadStoredString(SIDEBAR_COLLAPSED_KEY) === "1", false);
applyRightState(loadStoredString(RIGHT_COLLAPSED_KEY) === "1", false);

wireCollapsibleSections();
applyAuthView();
