package domain

import "time"

// Session 是 history 服务对外暴露的会话视图。
type Session struct {
	ID            string    `json:"id"`
	UserID        uint64    `json:"user_id"`
	AgentIDs      []string  `json:"agent_ids,omitempty"`
	Title         string    `json:"title"`
	Summary       string    `json:"summary"`
	LLMName       string    `json:"llm_name"`
	LastMessageAt time.Time `json:"last_message_at"`
	Status        uint8     `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
}

// Message 是 history 服务对外暴露的消息视图。
type Message struct {
	ID        uint64    `json:"id"`
	SessionID string    `json:"session_id"`
	Seq       uint64    `json:"seq"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	LLMName   string    `json:"llm_name"`
	Model     string    `json:"model"`
	CreatedAt time.Time `json:"created_at"`
}
