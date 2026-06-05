package model

import "time"

// ChatMessage 是会话历史的领域类型，对外暴露给 agent / service 使用。
//
// 与 dao 内部的 GORM record 结构不同：这里没有 GORM 标签，避免领域层依赖持久化细节。
type ChatMessage struct {
	ID        uint64
	UserID    string
	AgentID   string
	SessionID string
	Role      string
	Content   string
	LLMName   string
	Model     string
	CreatedAt time.Time
}
