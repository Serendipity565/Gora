package model

import "time"

// ModelSelection 记录 (user, agent, session) 三元组下当前选用的 LLM。
type ModelSelection struct {
	UserID    string
	AgentID   string
	SessionID string
	LLMName   string
	Model     string
	LLMIndex  int
	UpdatedAt time.Time
}
