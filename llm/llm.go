package llm

import "context"

// Role 表示对话消息的角色。
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message 表示一条 LLM 对话消息。
type Message struct {
	Role       Role       `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// ToolCall 表示 LLM 返回的一次工具调用。
type ToolCall struct {
	Index    *int   `json:"index,omitempty"`
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// LLM 定义大模型客户端需要实现的统一接口。
type LLM interface {
	Chat(ctx context.Context, messages []Message, tools []map[string]any) (*Message, error)
	ChatStream(ctx context.Context, messages []Message, tools []map[string]any) <-chan StreamChunk
}

// StreamChunk 表示流式 LLM 调用返回的一个增量片段。
type StreamChunk struct {
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls"`
	Done      bool       `json:"done"`
	Error     error      `json:"error,omitempty"`
}
