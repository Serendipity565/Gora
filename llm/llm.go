package llm

import "context"

// Role is the role of a chat message.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message is a chat message.
type Message struct {
	Role       Role       `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// ToolCall is a tool call returned by an LLM.
type ToolCall struct {
	Index    *int   `json:"index,omitempty"`
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// LLM defines the language model abstraction.
type LLM interface {
	Chat(ctx context.Context, messages []Message, tools []map[string]any) (*Message, error)
	ChatStream(ctx context.Context, messages []Message, tools []map[string]any) <-chan StreamChunk
}

// StreamChunk is an incremental output chunk from a streaming LLM call.
type StreamChunk struct {
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls"`
	Done      bool       `json:"done"`
	Error     error      `json:"error,omitempty"`
}
