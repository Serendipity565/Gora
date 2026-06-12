import DOMPurify from "dompurify";
import { marked } from "marked";

import type { AgentEvent, ToolInfo } from "./types";

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
 * 16px 的容差允许行高 / 滚动条精度上的小误差。
 */
export function scrollToBottomIfPinned(container: HTMLElement, threshold = 16): void {
  const distance = container.scrollHeight - container.clientHeight - container.scrollTop;
  if (distance <= threshold) {
    container.scrollTop = container.scrollHeight;
  }
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
  return { bubble, streamTarget: null, streamRaw: "", onPermission };
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
      const argsJSON = args !== undefined ? JSON.stringify(args, null, 2) : "";
      const summaryPreview = argsJSON ? truncatePreview(argsJSON) : "";
      const argsBody = argsJSON
        ? `<pre class="event-collapse-body"><code>${escapeHtml(argsJSON)}</code></pre>`
        : "";
      appendEventNode(
        handle.bubble,
        "event-tool-call event-collapse",
        `<details class="event-collapse-details">
           <summary class="event-collapse-summary">
             <span class="event-collapse-icon" aria-hidden="true">▸</span>
             <span class="event-collapse-title">🔧 调用工具 <strong>${escapeHtml(toolName)}</strong></span>
             ${summaryPreview ? `<span class="event-collapse-preview">${escapeHtml(summaryPreview)}</span>` : ""}
           </summary>
           ${argsBody}
         </details>`,
      );
      return sealStream(handle);
    }

    case "tool_result": {
      const tool = event.metadata?.["tool"];
      const header = tool
        ? `📥 工具 <strong>${escapeHtml(String(tool))}</strong> 返回`
        : "📥 工具返回";
      const content = event.content ?? "";
      const summaryPreview = truncatePreview(content);
      appendEventNode(
        handle.bubble,
        "event-tool-result event-collapse",
        `<details class="event-collapse-details">
           <summary class="event-collapse-summary">
             <span class="event-collapse-icon" aria-hidden="true">▸</span>
             <span class="event-collapse-title">${header}</span>
             ${summaryPreview ? `<span class="event-collapse-preview">${escapeHtml(summaryPreview)}</span>` : ""}
           </summary>
           <pre class="event-collapse-body">${escapeHtml(content)}</pre>
         </details>`,
      );
      return sealStream(handle);
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
