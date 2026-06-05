import "./style.css";

import { listAgents, listTools, streamChat } from "./api";
import type { AgentEvent } from "./types";
import {
  appendAgentBubble,
  appendUserBubble,
  applyEvent,
  renderAgentMeta,
  renderConnection,
  renderToolList,
  scrollToBottom,
  setAgentState,
} from "./ui";

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

let inFlight: AbortController | null = null;

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

async function bootstrap(): Promise<void> {
  pushWelcome();
  renderConnection(connectionDotEl, connectionLabelEl, "connecting", "连接中…");

  try {
    const [agents, tools] = await Promise.all([listAgents(), listTools()]);
    renderAgentMeta(modelNameEl, agentStateEl, agents);
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

async function sendMessage(message: string): Promise<void> {
  const trimmed = message.trim();
  if (!trimmed || inFlight) return;

  appendUserBubble(messagesEl, trimmed);
  userInputEl.value = "";
  autoResize();

  const counters = resetCounters();
  setProcessing(true);
  setAgentState(agentStateEl, "running");

  let handle = appendAgentBubble(messagesEl);
  inFlight = new AbortController();

  try {
    await streamChat(
      { message: trimmed },
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
userInputEl.focus();
