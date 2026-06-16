import DOMPurify from "dompurify";
import { marked } from "marked";

import type { AgentEvent, MessageItem, SessionItem, ToolInfo } from "./types";

// marked 配置：开启 GFM、把单换行视为 <br>，与 ChatGPT 风格的输出更接近。
marked.setOptions({
  gfm: true,
  breaks: true,
});

const escapeHtml = (text: string): string => {
  const div = document.createElement("div");
  div.textContent = text;
  return div.innerHTML;
};

/**
 * 给折叠 summary 用的单行预览：把多行内容压成一行，
 * 超出 120 字以省略号截断。原始内容由展开后的 <pre> 完整呈现。
 */
function truncatePreview(text: string, max = 120): string {
  const flat = text.replace(/\s+/g, " ").trim();
  if (flat.length <= max) return flat;
  return flat.slice(0, max).trimEnd() + "…";
}

/**
 * 把 Agent 输出的原始 markdown 渲染为安全 HTML。
 * 流式过程中也会被反复调用，因此用 marked 的同步模式 + DOMPurify。
 */
function renderMarkdown(raw: string): string {
  const html = marked.parse(raw, { async: false }) as string;
  return DOMPurify.sanitize(html, {
    ADD_ATTR: ["target", "rel"],
  });
}

export function scrollToBottom(container: HTMLElement): void {
  container.scrollTop = container.scrollHeight;
}

/**
 * 仅当用户当前已经处于（接近）底部时才滚到底。
 *
 * 用来在流式输出时跟随最新内容；如果用户主动往上翻看历史，就不要再把他拽回底部。
 * 48px 的容差允许行高 / 滚动条精度上的小误差。
 *
 * 注意：本函数会在 rAF 中再判一次距离，因此调用方在 DOM 变更前判断"是否已贴底"
 * 比依赖本函数自己更稳——DOM 变更后 scrollHeight 已经长出来了，距离会突然变大。
 */
export function scrollToBottomIfPinned(container: HTMLElement, threshold = 48): void {
  requestAnimationFrame(() => {
    const distance = container.scrollHeight - container.clientHeight - container.scrollTop;
    if (distance <= threshold) {
      container.scrollTop = container.scrollHeight;
    }
  });
}

/**
 * 判断容器当前是否处于（接近）底部。
 *
 * 给 main.ts 用来"在 DOM 变更前先采样、变更后据此决定是否贴底"。
 * 必须在 mutation 前调用，否则 scrollHeight 已经增长，距离失真。
 */
export function isPinnedToBottom(container: HTMLElement, threshold = 48): boolean {
  const distance = container.scrollHeight - container.clientHeight - container.scrollTop;
  return distance <= threshold;
}

/** 清空对话区，开启新会话时调用。 */
export function clearMessages(container: HTMLElement): void {
  container.innerHTML = "";
}

export function appendUserBubble(container: HTMLElement, text: string): void {
  // ChatGPT 风格：用户消息只有一个右对齐的 bubble，不带头像。
  const wrapper = document.createElement("div");
  wrapper.className = "message user";
  const bubble = document.createElement("div");
  bubble.className = "bubble";
  bubble.textContent = text; // 用 textContent 自动转义并保留换行
  bubble.style.whiteSpace = "pre-wrap";
  wrapper.appendChild(bubble);
  container.appendChild(wrapper);
  scrollToBottom(container);
}

export interface PermissionDecision {
  requestID: string;
  toolName: string;
  approve: boolean;
  remember: boolean;
}

export type PermissionHandler = (decision: PermissionDecision) => Promise<void>;

export interface AgentBubbleHandle {
  bubble: HTMLDivElement;
  /** 流式输出节点，按需创建。 */
  streamTarget: HTMLDivElement | null;
  /** 已累积的原始 markdown 文本（用于增量重渲染）。 */
  streamRaw: string;
  /** 收到 tool_permission_request 时调用的回调，由 main 注入。 */
  onPermission?: PermissionHandler;
  /**
   * 等待 result 的 tool_call 节点：以后端生成的 call_id 为键，
   * 一一对应。这是首选配对方式——不受并发耗时差影响。
   */
  pendingToolCallsByID: Map<string, HTMLDivElement>;
  /**
   * 兜底：没有 call_id 时按 tool 名 FIFO 配对（异常情况下才走这里）。
   */
  pendingToolCallsByName: Map<string, HTMLDivElement[]>;
}

export function appendAgentBubble(
  container: HTMLElement,
  onPermission?: PermissionHandler,
  modelLabel?: string,
): AgentBubbleHandle {
  const wrapper = document.createElement("div");
  wrapper.className = "message agent";
  // AI 原子图标（与 sidebar brand-icon / favicon 同款），保持视觉一致。
  wrapper.innerHTML = `
    <div class="avatar agent" aria-hidden="true">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" aria-hidden="true">
        <ellipse cx="12" cy="12" rx="9.5" ry="3.8" />
        <ellipse cx="12" cy="12" rx="9.5" ry="3.8" transform="rotate(60 12 12)" />
        <ellipse cx="12" cy="12" rx="9.5" ry="3.8" transform="rotate(120 12 12)" />
        <circle cx="20.4" cy="5.2" r="1" fill="currentColor" />
        <circle cx="3.6" cy="14.5" r="1" fill="currentColor" />
        <circle cx="18" cy="20" r="0.8" fill="currentColor" />
      </svg>
    </div>
  `;
  const bubble = document.createElement("div");
  bubble.className = "bubble";
  // 调试用：把当前模型名打到气泡顶部，方便观察哪条回答出自哪个模型。
  const label = (modelLabel ?? "").trim();
  if (label) {
    const badge = document.createElement("div");
    badge.className = "model-badge";
    badge.textContent = `model: ${label}`;
    bubble.appendChild(badge);
  }
  wrapper.appendChild(bubble);
  container.appendChild(wrapper);
  scrollToBottom(container);
  return {
    bubble,
    streamTarget: null,
    streamRaw: "",
    onPermission,
    pendingToolCallsByID: new Map<string, HTMLDivElement>(),
    pendingToolCallsByName: new Map<string, HTMLDivElement[]>(),
  };
}

export function appendEventNode(
  bubble: HTMLDivElement,
  className: string,
  innerHTML: string,
): HTMLDivElement {
  const node = document.createElement("div");
  node.className = `agent-event ${className}`;
  node.innerHTML = innerHTML;
  bubble.appendChild(node);
  return node;
}

/**
 * 把 SSE 事件应用到 Agent 气泡上，返回更新后的 handle（可能创建/复用流式节点）。
 *
 * 事件渲染顺序遵循"按到达顺序 append"。chunk 会创建一个流式段落 div，
 * 一旦中间出现非 chunk 事件（thinking / tool_call / tool_result / permission / error）
 * 就把当前流式段落"封口"——清空 handle.streamTarget，让下一段 chunk 新建一个
 * div 追加在事件节点之后，避免出现"模型回复跑到 tool 调用前面"。
 */
export function applyEvent(
  handle: AgentBubbleHandle,
  event: AgentEvent,
  counters: { event: number; tool: number; chunk: number },
): AgentBubbleHandle {
  // 任何非 chunk 事件都先把当前流式段落封口，
  // 这样下一段 chunk 会以新的 div 接在该事件节点之后。
  const sealStream = (next: AgentBubbleHandle): AgentBubbleHandle => {
    if (next.streamTarget) {
      next.streamTarget.classList.remove("typing-cursor");
      if (next.streamRaw) {
        next.streamTarget.innerHTML = renderMarkdown(next.streamRaw);
      }
    }
    return { ...next, streamTarget: null, streamRaw: "" };
  };

  switch (event.type) {
    case "thinking":
      appendEventNode(
        handle.bubble,
        "event-thinking",
        escapeHtml(event.content ?? "正在思考..."),
      );
      return sealStream(handle);

    case "tool_call": {
      counters.tool += 1;
      const args = event.metadata?.["args"];
      const toolName = event.content ?? "";
      const callID = typeof event.metadata?.["call_id"] === "string"
        ? (event.metadata!["call_id"] as string)
        : "";
      const argsJSON = args !== undefined ? JSON.stringify(args, null, 2) : "";
      const node = makeToolCallNode(toolName, argsJSON);
      handle.bubble.appendChild(node);
      // 优先用 call_id 注册（一对一精确配对，并发耗时不一致也不会错）。
      // 没有 id 才退回到 tool 名 FIFO（理论上不该走到这里）。
      if (callID) {
        handle.pendingToolCallsByID.set(callID, node);
      } else {
        const queue = handle.pendingToolCallsByName.get(toolName);
        if (queue) queue.push(node);
        else handle.pendingToolCallsByName.set(toolName, [node]);
      }
      return sealStream(handle);
    }

    case "tool_result": {
      const tool = event.metadata?.["tool"];
      const toolName = tool ? String(tool) : "";
      const callID = typeof event.metadata?.["call_id"] === "string"
        ? (event.metadata!["call_id"] as string)
        : "";
      const content = event.content ?? "";

      // 1. 先 sealStream（封口任何流式段落）——保证下面紧跟着插入的结果块顺序正确。
      const sealed = sealStream(handle);

      // 2. 找到本次 result 对应的 tool_call 节点。
      //    优先 call_id 直查；找不到再退到 tool 名 FIFO。
      let pairedCall: HTMLDivElement | null = null;
      if (callID) {
        pairedCall = sealed.pendingToolCallsByID.get(callID) ?? null;
        if (pairedCall) sealed.pendingToolCallsByID.delete(callID);
      }
      if (!pairedCall && toolName) {
        const queue = sealed.pendingToolCallsByName.get(toolName);
        if (queue && queue.length > 0) {
          pairedCall = queue.shift() ?? null;
          if (queue.length === 0) sealed.pendingToolCallsByName.delete(toolName);
        }
      }

      // 3. 创建结果节点（与 call 是兄弟，各自独立折叠），紧贴 call 之后。
      const resultNode = makeToolResultNode(toolName, content);
      attachResultAfterCall(sealed.bubble, pairedCall, resultNode);
      return sealed;
    }

    case "tool_permission_request": {
      const requestID = String(event.metadata?.["request_id"] ?? "");
      const toolName = String(event.metadata?.["tool"] ?? event.content ?? "");
      const args = event.metadata?.["args"];
      const argsHtml =
        args !== undefined
          ? `<pre class="permission-args">${escapeHtml(JSON.stringify(args, null, 2))}</pre>`
          : "";
      const node = appendEventNode(
        handle.bubble,
        "event-permission",
        `
          <div class="permission-header">
            🔐 模型请求调用工具 <strong>${escapeHtml(toolName)}</strong>，该工具当前已被你关闭。
          </div>
          ${argsHtml}
          <div class="permission-status">是否本次允许调用？</div>
          <div class="permission-actions">
            <button type="button" class="permission-btn allow" data-action="allow-once">允许一次</button>
            <button type="button" class="permission-btn allow-remember" data-action="allow-remember">允许并启用</button>
            <button type="button" class="permission-btn deny" data-action="deny">拒绝</button>
          </div>
        `,
      );

      const handlerCb = handle.onPermission;
      if (handlerCb && requestID) {
        node.querySelectorAll<HTMLButtonElement>(".permission-btn").forEach((btn) => {
          btn.addEventListener("click", () => {
            const action = btn.dataset["action"];
            if (!action) return;
            // 立刻锁定按钮，避免重复点击。
            node.querySelectorAll<HTMLButtonElement>(".permission-btn").forEach((b) => {
              b.disabled = true;
            });
            const decision: PermissionDecision = {
              requestID,
              toolName,
              approve: action !== "deny",
              remember: action === "allow-remember",
            };
            handlerCb(decision)
              .then(() => {
                markPermissionResolved(node, decision);
              })
              .catch((err: Error) => {
                markPermissionResolved(node, decision, err.message);
              });
          });
        });
      } else {
        // 缺少 request_id 或没有回调注册时，按钮置灰。
        node.querySelectorAll<HTMLButtonElement>(".permission-btn").forEach((b) => {
          b.disabled = true;
        });
      }
      return sealStream(handle);
    }

    case "chunk": {
      counters.chunk += 1;
      let target = handle.streamTarget;
      if (!target) {
        target = document.createElement("div");
        target.className = "agent-text typing-cursor";
        handle.bubble.appendChild(target);
      }
      const nextRaw = handle.streamRaw + (event.content ?? "");
      target.innerHTML = renderMarkdown(nextRaw);
      return { ...handle, streamTarget: target, streamRaw: nextRaw };
    }

    case "error":
      appendEventNode(
        handle.bubble,
        "event-error",
        `❌ ${escapeHtml(event.content ?? "未知错误")}`,
      );
      return sealStream(handle);

    case "done":
      // 收到 done 后做两件事：
      //   1. 移除流式光标动画，再渲染一次完整 markdown
      //   2. 把所有「正在思考...」之类的 thinking 提示替换为「✅ 已完成」
      if (handle.streamTarget) {
        handle.streamTarget.classList.remove("typing-cursor");
        // 流式过程中可能存在临时未闭合的 markdown，最终再渲染一次确保完整。
        if (handle.streamRaw) {
          handle.streamTarget.innerHTML = renderMarkdown(handle.streamRaw);
        }
      }
      handle.bubble.querySelectorAll(".event-thinking").forEach((node) => {
        node.classList.remove("event-thinking");
        node.classList.add("event-done");
        node.innerHTML = `<span class="event-done-icon">✨</span><span class="event-done-text">本轮思考已完成，回答已生成</span>`;
      });
      return handle;

    default:
      // 未知事件以原始内容降级展示，便于调试。
      appendEventNode(
        handle.bubble,
        "event-thinking",
        `· ${escapeHtml(event.content ?? event.type)}`,
      );
      return sealStream(handle);
  }
}

/**
 * 把 Agent 状态字符串映射为侧栏 chip 的 className。
 */
export function setAgentState(el: HTMLElement, state: string): void {
  const known = new Set(["idle", "running", "waiting", "done", "error"]);
  const cls = known.has(state) ? `state-${state}` : "state-idle";
  el.className = `agent-state ${cls}`;
  el.textContent = stateLabel(state);
}

function stateLabel(state: string): string {
  switch (state) {
    case "idle": return "空闲";
    case "running": return "思考中";
    case "waiting": return "等待中";
    case "done": return "已完成";
    case "error": return "错误";
    default: return state || "未知";
  }
}

export function renderToolList(container: HTMLElement, tools: ToolInfo[]): void {
  if (!tools.length) {
    container.innerHTML = `<div class="tool-item placeholder">没有已注册的工具</div>`;
    return;
  }
  const disabled = loadDisabledTools();
  container.innerHTML = "";
  for (const tool of tools) {
    const item = document.createElement("div");
    const isDisabled = disabled.has(tool.name);
    item.className = `tool-item tool-entry${isDisabled ? " disabled" : ""}`;
    const safeName = escapeHtml(tool.name);
    item.innerHTML = `
      <div class="tool-info">
        <div class="tool-name">${safeName}</div>
        <div class="tool-desc">${escapeHtml(tool.description)}</div>
      </div>
      <label class="tool-switch" title="启用 / 关闭工具" aria-label="启用或关闭 ${safeName}">
        <input type="checkbox" data-tool="${safeName}" ${isDisabled ? "" : "checked"} />
        <span class="tool-switch-slider"></span>
      </label>
    `;

    const checkbox = item.querySelector<HTMLInputElement>("input[type=checkbox]");
    checkbox?.addEventListener("change", () => {
      const set = loadDisabledTools();
      if (checkbox.checked) {
        set.delete(tool.name);
        item.classList.remove("disabled");
      } else {
        set.add(tool.name);
        item.classList.add("disabled");
      }
      saveDisabledTools(set);
      container.dispatchEvent(
        new CustomEvent("tool-toggle", {
          detail: { name: tool.name, enabled: checkbox.checked },
          bubbles: true,
        }),
      );
    });

    container.appendChild(item);
  }
}

const DISABLED_TOOLS_KEY = "gora.disabledTools";

function loadDisabledTools(): Set<string> {
  try {
    const raw = localStorage.getItem(DISABLED_TOOLS_KEY);
    if (!raw) return new Set();
    const parsed = JSON.parse(raw) as unknown;
    if (Array.isArray(parsed)) {
      return new Set(parsed.filter((x): x is string => typeof x === "string"));
    }
  } catch {
    // 解析失败时回退到空集，不阻塞 UI。
  }
  return new Set();
}

function saveDisabledTools(set: Set<string>): void {
  try {
    localStorage.setItem(DISABLED_TOOLS_KEY, JSON.stringify([...set]));
  } catch {
    // localStorage 不可用时静默忽略（如隐私模式）。
  }
}

/** 当前被关闭的工具名集合。 */
export function getDisabledTools(): Set<string> {
  return loadDisabledTools();
}

/**
 * 把工具从禁用集合中移除（用于"允许并启用"流程）。
 * 同时同步刷新 toolList 内对应开关的视觉状态。
 */
export function enableTool(toolName: string, toolListRoot?: HTMLElement | null): void {
  const set = loadDisabledTools();
  if (!set.has(toolName)) return;
  set.delete(toolName);
  saveDisabledTools(set);
  if (toolListRoot) {
    toolListRoot
      .querySelectorAll<HTMLInputElement>(`input[type=checkbox][data-tool="${cssEscape(toolName)}"]`)
      .forEach((input) => {
        input.checked = true;
        input.closest(".tool-entry")?.classList.remove("disabled");
      });
  }
}

function cssEscape(value: string): string {
  // CSS.escape 在所有现代浏览器都有，但仍兜底一下。
  if (typeof CSS !== "undefined" && typeof CSS.escape === "function") {
    return CSS.escape(value);
  }
  return value.replace(/[^a-zA-Z0-9_-]/g, "\\$&");
}

/**
 * 把权限气泡更新为"已处理"状态。
 */
function markPermissionResolved(
  node: HTMLElement,
  decision: PermissionDecision,
  errorMessage?: string,
): void {
  const status = node.querySelector<HTMLDivElement>(".permission-status");
  const actions = node.querySelector<HTMLDivElement>(".permission-actions");
  if (errorMessage) {
    if (status) status.textContent = `❌ 提交失败：${errorMessage}`;
    node.querySelectorAll<HTMLButtonElement>(".permission-btn").forEach((b) => {
      b.disabled = false;
    });
    return;
  }
  if (status) {
    if (decision.approve && decision.remember) {
      status.textContent = `✅ 已允许并重新启用工具 ${decision.toolName}`;
    } else if (decision.approve) {
      status.textContent = `✅ 已允许本次调用 ${decision.toolName}`;
    } else {
      status.textContent = `🚫 已拒绝调用 ${decision.toolName}`;
    }
  }
  if (actions) actions.remove();
  node.classList.add("permission-resolved");
  node.classList.toggle("approved", decision.approve);
  node.classList.toggle("denied", !decision.approve);
}

/**
 * 让侧栏中带有 data-section 的 section 支持点击折叠/展开，
 * 并将状态记忆到 localStorage 中。
 */
export function wireCollapsibleSections(root: ParentNode = document): void {
  const sections = root.querySelectorAll<HTMLElement>(".collapsible-section");
  sections.forEach((section) => {
    const header = section.querySelector<HTMLButtonElement>(".collapsible-header");
    if (!header) return;
    // 防止重复绑定
    if (header.dataset["wired"] === "1") return;
    header.dataset["wired"] = "1";

    const sectionKey = section.dataset["section"] ?? "";
    const storageKey = sectionKey ? `gora.collapsed.${sectionKey}` : "";

    if (storageKey) {
      try {
        if (localStorage.getItem(storageKey) === "1") {
          section.classList.add("collapsed");
          header.setAttribute("aria-expanded", "false");
        }
      } catch {
        // 忽略
      }
    }

    header.addEventListener("click", () => {
      const collapsed = section.classList.toggle("collapsed");
      header.setAttribute("aria-expanded", collapsed ? "false" : "true");
      if (storageKey) {
        try {
          if (collapsed) localStorage.setItem(storageKey, "1");
          else localStorage.removeItem(storageKey);
        } catch {
          // 忽略
        }
      }
    });
  });
}

export function renderModelMeta(
  modelEl: HTMLElement,
  stateEl: HTMLElement,
): void {
  modelEl.textContent = "model: 未注册";
  setAgentState(stateEl, "idle");
}

export function renderConnection(
  dot: HTMLElement,
  label: HTMLElement,
  status: "connecting" | "online" | "error",
  text: string,
): void {
  dot.className = "status-dot";
  if (status === "online") dot.classList.add("online");
  if (status === "error") dot.classList.add("error");
  label.textContent = text;
}

/* ============================================================
 * 会话列表 (sidebar) + 历史消息渲染
 * ========================================================== */

export interface SessionListCallbacks {
  /** 用户点击某个 session 时触发；activeSessionID 已经在 main 那边切换好。 */
  onSelect: (session: SessionItem) => void;
}

/**
 * 把会话列表渲染到 sidebar。
 *
 * - 点击某行触发 cb.onSelect，main 负责切换 currentSessionID 与拉历史；
 * - activeSessionID 命中的行高亮；
 * - 空列表显示占位文本。
 */
export function renderSessionList(
  container: HTMLElement,
  sessions: SessionItem[],
  activeSessionID: string,
  cb: SessionListCallbacks,
): void {
  if (!sessions.length) {
    container.innerHTML = `<div class="history-placeholder">暂无历史会话</div>`;
    return;
  }
  container.innerHTML = "";
  for (const session of sessions) {
    const item = document.createElement("button");
    item.type = "button";
    item.className = "history-item";
    if (session.id === activeSessionID) item.classList.add("active");
    item.dataset["sessionId"] = session.id;

    const titleText = session.title || "未命名会话";
    const subtitleText = formatRelativeTime(session.last_message_at) || formatRelativeTime(session.created_at);

    item.innerHTML = `
      <div class="history-item-title" title="${escapeHtml(titleText)}">${escapeHtml(titleText)}</div>
      ${subtitleText ? `<div class="history-item-meta">${escapeHtml(subtitleText)}</div>` : ""}
    `;
    item.addEventListener("click", () => cb.onSelect(session));
    container.appendChild(item);
  }
}

/** 把列表里某条 session 标记为选中（不重渲染整个列表）。 */
export function markActiveSession(container: HTMLElement, sessionID: string): void {
  container.querySelectorAll<HTMLElement>(".history-item").forEach((item) => {
    item.classList.toggle("active", item.dataset["sessionId"] === sessionID);
  });
}

/**
 * 把后端拉到的历史消息渲染到对话区。
 *
 * 与 streamChat 流式输出不同，这里是"一锤子渲染"：
 * 用户消息照原样转义渲染；assistant 消息按 markdown 解析；
 * tool / system 暂不显示在主对话流（避免噪音）。
 */
/**
 * 解析一条 role=tool 的历史消息。
 *
 * 后端约定（runner.go）每条 tool 行都在 `ToolCalls` JSON 里写：
 *   call:   {tool, call_id, args}
 *   result: {tool, call_id, kind: "result"}
 *
 * call_id 是把 call 与 result 一一对应的唯一钥匙（前端不依赖到达顺序 / 名字 FIFO）。
 * tool_calls 缺失或 JSON 损坏的行被视为脏数据，返回 null。
 */
function parseToolContent(m: MessageItem): { kind: "call" | "result"; name: string; callID: string; detail: string } | null {
  if (!m.tool_calls) return null;
  let meta: Record<string, unknown>;
  try {
    meta = JSON.parse(m.tool_calls) as Record<string, unknown>;
  } catch {
    return null;
  }

  const toolName = typeof meta["tool"] === "string" ? (meta["tool"] as string) : "";
  const callID = typeof meta["call_id"] === "string" ? (meta["call_id"] as string) : "";
  if (meta["kind"] === "result") {
    return { kind: "result", name: toolName, callID, detail: (m.content || "").trim() };
  }
  const args = meta["args"];
  return {
    kind: "call",
    name: toolName,
    callID,
    detail: args === undefined ? "" : JSON.stringify(args, null, 2),
  };
}

export function renderHistoryMessages(
  container: HTMLElement,
  messages: MessageItem[],
): void {
  clearMessages(container);

  // 一个用户提问 + 跟随的 assistant/tool 流是"一个 turn"，
  // 渲染到同一个 agent 气泡里——和 live 模式视觉一致。
  // 只有遇到下一条 user 消息才换新气泡。
  let agentHandle: AgentBubbleHandle | null = null;
  let pendingTextNode: HTMLDivElement | null = null; // 当前 turn 内最近一段 assistant 文本节点
  let lastAssistantModel = ""; // 用于第一次开 agentHandle 时给气泡贴模型徽标

  const ensureAgentBubble = (): AgentBubbleHandle => {
    if (!agentHandle) {
      agentHandle = appendAgentBubble(container, undefined, lastAssistantModel);
    }
    return agentHandle;
  };

  for (const m of messages) {
    const role = (m.role || "").toLowerCase();

    if (role === "user") {
      // 重置 turn：下一条 assistant/tool 会开新 bubble
      agentHandle = null;
      pendingTextNode = null;
      lastAssistantModel = "";
      appendUserBubble(container, m.content);
      continue;
    }

    if (role === "system") {
      // system 角色不出现在新数据里，跳过即可。
      continue;
    }

    if (role === "assistant") {
      // 第一次见到这个 turn 的 assistant 时记下模型名给气泡用
      if (!agentHandle) {
        lastAssistantModel = m.model || m.llm_name || "";
      }
      const handle = ensureAgentBubble();
      const text = document.createElement("div");
      text.className = "agent-text";
      text.innerHTML = renderMarkdown(m.content || "");
      handle.bubble.appendChild(text);
      pendingTextNode = text;
      continue;
    }

    if (role === "tool") {
      const handle = ensureAgentBubble();
      const parsed = parseToolContent(m);
      if (!parsed) continue;

      if (parsed.kind === "call") {
        const callNode = makeToolCallNode(parsed.name, parsed.detail);
        handle.bubble.appendChild(callNode);
        // 优先按 call_id 注册，没有 id 才退到 tool 名 FIFO（异常路径）。
        if (parsed.callID) {
          handle.pendingToolCallsByID.set(parsed.callID, callNode);
        } else if (parsed.name) {
          const queue = handle.pendingToolCallsByName.get(parsed.name);
          if (queue) queue.push(callNode);
          else handle.pendingToolCallsByName.set(parsed.name, [callNode]);
        }
        // tool 块出现 → 当前 assistant 段落已经"封口"，
        // 下一条 assistant 文本必须新建一个节点（而不是接到旧段落里）。
        pendingTextNode = null;
        continue;
      }

      // result：先按 call_id 直查，缺失才退到 tool 名 FIFO
      let pairedCall: HTMLDivElement | null = null;
      if (parsed.callID) {
        pairedCall = handle.pendingToolCallsByID.get(parsed.callID) ?? null;
        if (pairedCall) handle.pendingToolCallsByID.delete(parsed.callID);
      }
      if (!pairedCall && parsed.name) {
        const queue = handle.pendingToolCallsByName.get(parsed.name);
        if (queue && queue.length > 0) {
          pairedCall = queue.shift() ?? null;
          if (queue.length === 0) handle.pendingToolCallsByName.delete(parsed.name);
        }
      }
      const resultNode = makeToolResultNode(parsed.name, parsed.detail);
      attachResultAfterCall(handle.bubble, pairedCall, resultNode);
      pendingTextNode = null;
      continue;
    }
  }

  // 让 lint 看得见这个变量被读过——pendingTextNode 在未来扩展（如把
  // 紧邻的多条 assistant 合并到同一段落）时是个挂点；这里显式标记一下。
  void pendingTextNode;

  scrollToBottom(container);
}

/* ============================================================
 * tool_call / tool_result 节点的"纯构造器"
 *
 * 这两个 helper 同时被 live 模式（applyEvent）与历史模式
 * （renderHistoryMessages）调用——把"长什么样"集中在这里，
 * 任何视觉调整改一处即可，不会让两条路径渐行渐远。
 *
 * 返回的节点尚未挂到 DOM 上，调用方负责 append / insertBefore。
 * ========================================================== */
function makeToolCallNode(toolName: string, argsJSON: string): HTMLDivElement {
  const summaryPreview = argsJSON ? truncatePreview(argsJSON) : "";
  const argsBody = argsJSON
    ? `<pre class="event-collapse-body"><code>${escapeHtml(argsJSON)}</code></pre>`
    : "";
  const node = document.createElement("div");
  node.className = "agent-event event-tool-call event-collapse";
  node.innerHTML = `<details class="event-collapse-details">
       <summary class="event-collapse-summary">
         <span class="event-collapse-chevron" aria-hidden="true">▶</span>
         <span class="event-collapse-badge">调用</span>
         <span class="event-collapse-title">🔧 <strong>${escapeHtml(toolName)}</strong></span>
         ${summaryPreview ? `<span class="event-collapse-preview">${escapeHtml(summaryPreview)}</span>` : ""}
         <span class="event-collapse-hint">点击折叠 / 展开</span>
       </summary>
       ${argsBody}
     </details>`;
  return node;
}

function makeToolResultNode(toolName: string, content: string): HTMLDivElement {
  const summaryPreview = truncatePreview(content);
  const resultBody = `<pre class="event-collapse-body">${escapeHtml(content)}</pre>`;
  const label = toolName
    ? `📥 <strong>${escapeHtml(toolName)}</strong>`
    : "📥 工具返回";
  const node = document.createElement("div");
  node.className = "agent-event event-tool-result event-collapse";
  node.innerHTML = `<details class="event-collapse-details">
       <summary class="event-collapse-summary">
         <span class="event-collapse-chevron" aria-hidden="true">▶</span>
         <span class="event-collapse-badge result">返回</span>
         <span class="event-collapse-title">${label}</span>
         ${summaryPreview ? `<span class="event-collapse-preview">${escapeHtml(summaryPreview)}</span>` : ""}
         <span class="event-collapse-hint">点击折叠 / 展开</span>
       </summary>
       ${resultBody}
     </details>`;
  return node;
}

/** 把 result 节点紧贴对应 call 节点之后插入，并让二者首尾相连。 */
function attachResultAfterCall(bubble: HTMLDivElement, call: HTMLDivElement | null, result: HTMLDivElement): void {
  if (call && call.parentNode === bubble) {
    bubble.insertBefore(result, call.nextSibling);
    call.classList.add("event-tool-call-paired");
  } else {
    bubble.appendChild(result);
  }
}

/** 把 ISO8601 时间戳格式化为相对时间（"刚刚 / 5 分钟前 / 昨天 / 3 天前 / MM-DD"）。 */
function formatRelativeTime(iso: string): string {
  if (!iso) return "";
  const ts = Date.parse(iso);
  if (Number.isNaN(ts)) return "";
  const now = Date.now();
  const diff = now - ts;
  if (diff < 0) return "刚刚";
  const minute = 60_000;
  const hour = 60 * minute;
  const day = 24 * hour;

  if (diff < minute) return "刚刚";
  if (diff < hour) return `${Math.floor(diff / minute)} 分钟前`;
  if (diff < day) return `${Math.floor(diff / hour)} 小时前`;
  if (diff < 2 * day) return "昨天";
  if (diff < 7 * day) return `${Math.floor(diff / day)} 天前`;

  const d = new Date(ts);
  const month = String(d.getMonth() + 1).padStart(2, "0");
  const day2 = String(d.getDate()).padStart(2, "0");
  // 跨年时显示完整日期，避免歧义。
  if (d.getFullYear() !== new Date().getFullYear()) {
    return `${d.getFullYear()}-${month}-${day2}`;
  }
  return `${month}-${day2}`;
}
