package model

import (
	"time"

	"gorm.io/gorm"
)

// ChatMessage 是会话历史的领域类型，对外暴露给 agent / service 使用。
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
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"` // 软删除字段
}
