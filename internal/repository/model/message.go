package model

import (
	"time"

	"gorm.io/datatypes"
)

type MessageRole string

const (
	RoleSystem    MessageRole = "system"
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
	RoleTool      MessageRole = "tool"
)

// Message 是 Session 下的一条对话消息。
//
// SessionID 与 Session.ID 类型对齐（字符串），方便业务侧自由生成 session id；
// 联合唯一索引 (session_id, seq) 保证同一 session 下消息序号唯一。
type Message struct {
	ID        uint64         `gorm:"primaryKey;autoIncrement"`
	SessionID string         `gorm:"size:64;not null;uniqueIndex:idx_session_seq,priority:1;index:idx_session_created,priority:1"`
	Seq       uint64         `gorm:"uniqueIndex:idx_session_seq,priority:2;not null"`
	UserID    uint64         `gorm:"not null;default:0"`
	AgentID   string         `gorm:"size:191;not null;default:''"`
	Role      MessageRole    `gorm:"size:20;not null"`
	Content   string         `gorm:"type:text"`
	LLMName   string         `gorm:"size:191;not null;default:''"`
	Model     string         `gorm:"size:191;not null;default:''"`
	ToolCalls datatypes.JSON `gorm:"type:json"`
	CreatedAt time.Time      `gorm:"index:idx_session_created,priority:2"`
}

func (Message) TableName() string {
	return "message"
}
