package response

// SessionItem 是 /api/sessions 返回的一条会话视图。
type SessionItem struct {
	ID            string   `json:"id"`
	AgentIDs      []string `json:"agent_ids,omitempty"`
	Title         string   `json:"title"`
	Summary       string   `json:"summary"`
	LLMName       string   `json:"llm_name"`
	LastMessageAt string   `json:"last_message_at"`
	Status        uint8    `json:"status"`
	CreatedAt     string   `json:"created_at"`
}

// MessageItem 是 /api/sessions/{sessionId}/messages 返回的一条消息视图。
type MessageItem struct {
	ID        uint64 `json:"id"`
	SessionID string `json:"session_id"`
	Seq       uint64 `json:"seq"`
	Role      string `json:"role"`
	Content   string `json:"content"`
	ToolCalls string `json:"tool_calls,omitempty"`
	LLMName   string `json:"llm_name"`
	Model     string `json:"model"`
	CreatedAt string `json:"created_at"`
}

// SessionsResp 是 /api/sessions 列表响应体。
type SessionsResp struct {
	Sessions []SessionItem `json:"sessions"`
}

// MessagesResp 是 /api/sessions/{sessionId}/messages 列表响应体。
type MessagesResp struct {
	Messages []MessageItem `json:"messages"`
}
