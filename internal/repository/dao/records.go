package dao

import (
	"time"

	"github.com/Serendipity565/gora/internal/repository/model"
)

// modelSelectionRecord 是 GORM 持久化结构，仅 dao 内部使用。
type modelSelectionRecord struct {
	UserID    string `gorm:"primaryKey;size:191;column:user_id"`
	AgentID   string `gorm:"primaryKey;size:191;column:agent_id"`
	SessionID string `gorm:"primaryKey;size:191;column:session_id"`
	LLMName   string `gorm:"size:191;not null;default:'';column:llm_name"`
	Model     string `gorm:"size:191;not null;column:model"`
	LLMIndex  int    `gorm:"not null;column:llm_index"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (modelSelectionRecord) TableName() string {
	return "conversation_model_selections"
}

// chatMessageRecord 是 GORM 持久化结构，仅 dao 内部使用。
type chatMessageRecord struct {
	ID        uint64 `gorm:"primaryKey;autoIncrement;column:id"`
	UserID    string `gorm:"index:idx_conversation_messages_lookup,priority:1;size:191;not null;column:user_id"`
	AgentID   string `gorm:"index:idx_conversation_messages_lookup,priority:2;size:191;not null;column:agent_id"`
	SessionID string `gorm:"index:idx_conversation_messages_lookup,priority:3;size:191;not null;column:session_id"`
	Role      string `gorm:"size:32;not null;column:role"`
	Content   string `gorm:"type:text;not null;column:content"`
	LLMName   string `gorm:"size:191;not null;default:'';column:llm_name"`
	Model     string `gorm:"size:191;not null;default:'';column:model"`
	CreatedAt time.Time
}

func (chatMessageRecord) TableName() string {
	return "conversation_messages"
}

func modelSelectionRecordFromSelection(selection model.ModelSelection) modelSelectionRecord {
	return modelSelectionRecord{
		UserID:    selection.UserID,
		AgentID:   selection.AgentID,
		SessionID: selection.SessionID,
		LLMName:   selection.LLMName,
		Model:     selection.Model,
		LLMIndex:  selection.LLMIndex,
	}
}

func (r modelSelectionRecord) toSelection() model.ModelSelection {
	return model.ModelSelection{
		UserID:    r.UserID,
		AgentID:   r.AgentID,
		SessionID: r.SessionID,
		LLMName:   r.LLMName,
		Model:     r.Model,
		LLMIndex:  r.LLMIndex,
		UpdatedAt: r.UpdatedAt,
	}
}

func chatMessageRecordFromMessage(message model.ChatMessage) chatMessageRecord {
	return chatMessageRecord{
		UserID:    message.UserID,
		AgentID:   message.AgentID,
		SessionID: message.SessionID,
		Role:      message.Role,
		Content:   message.Content,
		LLMName:   message.LLMName,
		Model:     message.Model,
	}
}

func (r chatMessageRecord) toMessage() model.ChatMessage {
	return model.ChatMessage{
		ID:        r.ID,
		UserID:    r.UserID,
		AgentID:   r.AgentID,
		SessionID: r.SessionID,
		Role:      r.Role,
		Content:   r.Content,
		LLMName:   r.LLMName,
		Model:     r.Model,
		CreatedAt: r.CreatedAt,
	}
}
