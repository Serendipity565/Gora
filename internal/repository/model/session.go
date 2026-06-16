package model

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type SessionStatus uint8

const (
	SessionStatusActive SessionStatus = 1
	SessionStatusClosed SessionStatus = 2
)

// Session 是一次多轮对话的元数据。ID 用业务侧生成的字符串（UUID / nanoid 等），
// 兼顾 MySQL 主键定位与 Redis / runner 内存中以字符串作 key 的现状。
type Session struct {
	ID            string         `gorm:"primaryKey;size:64;not null"`
	UserID        uint64         `gorm:"index;not null"`
	AgentIDs      datatypes.JSON `gorm:"type:json"` // 参与 Agent ID 列表，如 ["agent-a","agent-b"]
	Title         string         `gorm:"size:255;not null;default:'新对话'"`
	Summary       string         `gorm:"type:text"`
	LLMName       string         `gorm:"size:191;not null;default:''"` // 当前 session 选中的 LLM 名（项目内唯一标识）
	LastMessageAt time.Time      `gorm:"index"`
	Status        SessionStatus  `gorm:"default:1;not null"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
	DeletedAt     gorm.DeletedAt `gorm:"index"`
}

func (Session) TableName() string {
	return "session"
}
