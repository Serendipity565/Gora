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
  session_id?: string;
  /** 用户在前端 UI 中关闭的工具名列表，后端会拒绝模型调用其中任一工具。 */
  disabled_tools?: string[];
}

/**
 * 与 GET /api/sessions 返回的 SessionItem 对齐。
 *
 * - `id`         业务侧生成的 session id（与 chat / message 接口共用）
 * - `agent_ids`  该 session 涉及到的 Agent ID 列表（多 Agent 协作时用）
 * - `last_message_at` / `created_at`  ISO8601 字符串
 */
export interface SessionItem {
  id: string;
  agent_ids?: string[];
  title: string;
  summary: string;
  llm_name: string;
  last_message_at: string;
  status: number;
  created_at: string;
}

/**
 * 与 GET /api/sessions/{id}/messages 返回的 MessageItem 对齐。
 *
 * `role` 取值：'user' | 'assistant' | 'system' | 'tool'。
 */
export interface MessageItem {
  id: number;
  session_id: string;
  seq: number;
  role: string;
  content: string;
  tool_calls?: string;
  llm_name: string;
  model: string;
  created_at: string;
}
