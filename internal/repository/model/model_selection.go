package model

import (
	"time"

	"gorm.io/gorm"
)

// ModelSelection 记录 (user, agent, session) 三元组下当前选用的 LLM。
type ModelSelection struct {
	ID        uint64 `gorm:"primaryKey;autoIncrement"`
	UserID    string
	AgentID   string
	SessionID string
	LLMName   string
	Model     string
	LLMIndex  int
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"` // 软删除字段
}
