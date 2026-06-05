/**
 * 与 Go 后端 web 包对齐的事件类型定义。
 * 后端通过 SSE 推送 agent.Event，事件 type 与 agent/event.go 中保持一致。
 */
export type AgentEventType =
  | "thinking"
  | "tool_call"
  | "tool_result"
  | "tool_permission_request"
  | "chunk"
  | "done"
  | "error"
  | "unknown";

export interface AgentEvent {
  type: AgentEventType;
  agent_id?: string;
  content?: string;
  timestamp?: string;
  metadata?: Record<string, unknown>;
}

export interface AgentInfo {
  id: string;
  state: string;
  type: string;
  model?: string;
}

export interface ToolInfo {
  name: string;
  description: string;
  parameters?: Record<string, unknown>;
}

export interface ModelInfo {
  index: number;
  name?: string;
  provider?: string;
  model: string;
  /** 适合在下拉里展示的人类可读名称。 */
  display: string;
}

export interface ChatRequest {
  message: string;
  agent_id?: string;
  session_id?: string;
  /** 用户在前端 UI 中关闭的工具名列表，后端会拒绝模型调用其中任一工具。 */
  disabled_tools?: string[];
}
