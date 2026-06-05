import DOMPurify from "dompurify";
import { marked } from "marked";

import type { AgentEvent, AgentInfo, ToolInfo } from "./types";

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

export function appendUserBubble(container: HTMLElement, text: string): void {
  const wrapper = document.createElement("div");
  wrapper.className = "message user";
  wrapper.innerHTML = `
    <div class="avatar user">👤</div>
    <div class="bubble">${escapeHtml(text)}</div>
  `;
  container.appendChild(wrapper);
  scrollToBottom(container);
}

export interface AgentBubbleHandle {
  bubble: HTMLDivElement;
  /** 流式输出节点，按需创建。 */
  streamTarget: HTMLDivElement | null;
  /** 已累积的原始 markdown 文本（用于增量重渲染）。 */
  streamRaw: string;
}

export function appendAgentBubble(container: HTMLElement): AgentBubbleHandle {
  const wrapper = document.createElement("div");
  wrapper.className = "message agent";
  wrapper.innerHTML = `<div class="avatar agent">🦍</div>`;
  const bubble = document.createElement("div");
  bubble.className = "bubble";
  wrapper.appendChild(bubble);
  container.appendChild(wrapper);
  scrollToBottom(container);
  return { bubble, streamTarget: null, streamRaw: "" };
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
 */
export function applyEvent(
  handle: AgentBubbleHandle,
  event: AgentEvent,
  counters: { event: number; tool: number; chunk: number },
): AgentBubbleHandle {
  switch (event.type) {
    case "thinking":
      appendEventNode(
        handle.bubble,
        "event-thinking",
        escapeHtml(event.content ?? "正在思考..."),
      );
      return handle;

    case "tool_call": {
      counters.tool += 1;
      const args = event.metadata?.["args"];
      const argsHtml =
        args !== undefined
          ? `<br><code>${escapeHtml(JSON.stringify(args, null, 2))}</code>`
          : "";
      appendEventNode(
        handle.bubble,
        "event-tool-call",
        `🔧 调用工具 <strong>${escapeHtml(event.content ?? "")}</strong>${argsHtml}`,
      );
      return handle;
    }

    case "tool_result": {
      const tool = event.metadata?.["tool"];
      const header = tool ? `📥 工具 <strong>${escapeHtml(String(tool))}</strong> 返回` : "📥 工具返回";
      appendEventNode(
        handle.bubble,
        "event-tool-result",
        `${header}\n${escapeHtml(event.content ?? "")}`,
      );
      return handle;
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
      return handle;

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
      return handle;
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
  container.innerHTML = "";
  for (const tool of tools) {
    const item = document.createElement("div");
    item.className = "tool-item";
    item.innerHTML = `
      <div class="tool-name">${escapeHtml(tool.name)}</div>
      <div class="tool-desc">${escapeHtml(tool.description)}</div>
    `;
    container.appendChild(item);
  }
}

export function renderAgentMeta(
  modelEl: HTMLElement,
  stateEl: HTMLElement,
  agents: AgentInfo[],
): void {
  const first = agents[0];
  if (!first) {
    modelEl.textContent = "model: 未注册";
    setAgentState(stateEl, "idle");
    return;
  }
  modelEl.textContent = first.model ? `model: ${first.model}` : `agent: ${first.id}`;
  setAgentState(stateEl, first.state ?? "idle");
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
