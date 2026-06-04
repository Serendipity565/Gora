package storage

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const LocalUserID = "local"

type ModelSelection struct {
	UserID    string
	AgentID   string
	SessionID string
	LLMName   string
	Model     string
	LLMIndex  int
	UpdatedAt time.Time
}

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

type ModelSelectionStore interface {
	Get(ctx context.Context, userID, agentID, sessionID string) (ModelSelection, bool, error)
	Save(ctx context.Context, selection ModelSelection) error
	Close() error
}

type ChatHistoryStore interface {
	AppendMessages(ctx context.Context, messages []ChatMessage) error
	ListRecentMessages(ctx context.Context, userID, agentID, sessionID string, limit int) ([]ChatMessage, error)
}

type DatabaseStore interface {
	ModelSelectionStore
	ChatHistoryStore
}

type NoopModelSelectionStore struct{}

func OpenModelSelectionStore(ctx context.Context, databaseURL string) (DatabaseStore, error) {
	databaseURL = strings.TrimSpace(databaseURL)
	if databaseURL == "" {
		return NoopModelSelectionStore{}, nil
	}
	return NewGormModelSelectionStore(ctx, databaseURL)
}

func (NoopModelSelectionStore) Get(context.Context, string, string, string) (ModelSelection, bool, error) {
	return ModelSelection{}, false, nil
}

func (NoopModelSelectionStore) Save(context.Context, ModelSelection) error {
	return nil
}

func (NoopModelSelectionStore) Close() error {
	return nil
}

func (NoopModelSelectionStore) AppendMessages(context.Context, []ChatMessage) error {
	return nil
}

func (NoopModelSelectionStore) ListRecentMessages(context.Context, string, string, string, int) ([]ChatMessage, error) {
	return nil, nil
}

type GormModelSelectionStore struct {
	db *gorm.DB
}

func NewGormModelSelectionStore(ctx context.Context, databaseURL string) (*GormModelSelectionStore, error) {
	db, err := gorm.Open(mysql.Open(strings.TrimSpace(databaseURL)), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get mysql db handle: %w", err)
	}
	sqlDB.SetMaxOpenConns(5)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)

	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("ping mysql: %w", err)
	}

	store := &GormModelSelectionStore{db: db}
	if err := store.ensureSchema(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	return store, nil
}

func (s *GormModelSelectionStore) Get(ctx context.Context, userID, agentID, sessionID string) (ModelSelection, bool, error) {
	selection := ModelSelection{
		UserID:    normalizeKey(userID, LocalUserID),
		AgentID:   strings.TrimSpace(agentID),
		SessionID: strings.TrimSpace(sessionID),
	}
	if selection.AgentID == "" {
		return ModelSelection{}, false, fmt.Errorf("agent id cannot be empty")
	}
	if selection.SessionID == "" {
		return ModelSelection{}, false, fmt.Errorf("session id cannot be empty")
	}

	var record modelSelectionRecord
	err := s.db.WithContext(ctx).
		Where("user_id = ? AND agent_id = ? AND session_id = ?", selection.UserID, selection.AgentID, selection.SessionID).
		First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ModelSelection{}, false, nil
	}
	if err != nil {
		return ModelSelection{}, false, fmt.Errorf("get model selection: %w", err)
	}
	return record.toSelection(), true, nil
}

func (s *GormModelSelectionStore) Save(ctx context.Context, selection ModelSelection) error {
	selection.UserID = normalizeKey(selection.UserID, LocalUserID)
	selection.AgentID = strings.TrimSpace(selection.AgentID)
	selection.SessionID = strings.TrimSpace(selection.SessionID)
	selection.LLMName = strings.TrimSpace(selection.LLMName)
	selection.Model = strings.TrimSpace(selection.Model)

	if selection.AgentID == "" {
		return fmt.Errorf("agent id cannot be empty")
	}
	if selection.SessionID == "" {
		return fmt.Errorf("session id cannot be empty")
	}
	if selection.Model == "" {
		return fmt.Errorf("model cannot be empty")
	}
	if selection.LLMIndex < 0 {
		return fmt.Errorf("llm index cannot be negative")
	}

	record := modelSelectionRecordFromSelection(selection)
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "user_id"},
			{Name: "agent_id"},
			{Name: "session_id"},
		},
		DoUpdates: clause.AssignmentColumns([]string{"llm_name", "model", "llm_index", "updated_at"}),
	}).Create(&record).Error; err != nil {
		return fmt.Errorf("save model selection: %w", err)
	}
	return nil
}

func (s *GormModelSelectionStore) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func (s *GormModelSelectionStore) ensureSchema(ctx context.Context) error {
	if err := s.db.WithContext(ctx).AutoMigrate(&modelSelectionRecord{}, &chatMessageRecord{}); err != nil {
		return fmt.Errorf("ensure storage schema: %w", err)
	}
	return nil
}

func (s *GormModelSelectionStore) AppendMessages(ctx context.Context, messages []ChatMessage) error {
	records := make([]chatMessageRecord, 0, len(messages))
	for _, message := range messages {
		message.UserID = normalizeKey(message.UserID, LocalUserID)
		message.AgentID = strings.TrimSpace(message.AgentID)
		message.SessionID = strings.TrimSpace(message.SessionID)
		message.Role = strings.ToLower(strings.TrimSpace(message.Role))
		message.Content = strings.TrimSpace(message.Content)
		message.LLMName = strings.TrimSpace(message.LLMName)
		message.Model = strings.TrimSpace(message.Model)

		if message.AgentID == "" {
			return fmt.Errorf("agent id cannot be empty")
		}
		if message.SessionID == "" {
			return fmt.Errorf("session id cannot be empty")
		}
		if message.Role == "" {
			return fmt.Errorf("role cannot be empty")
		}
		if message.Content == "" {
			continue
		}
		records = append(records, chatMessageRecordFromMessage(message))
	}
	if len(records) == 0 {
		return nil
	}

	if err := s.db.WithContext(ctx).Create(&records).Error; err != nil {
		return fmt.Errorf("append chat messages: %w", err)
	}
	return nil
}

func (s *GormModelSelectionStore) ListRecentMessages(ctx context.Context, userID, agentID, sessionID string, limit int) ([]ChatMessage, error) {
	userID = normalizeKey(userID, LocalUserID)
	agentID = strings.TrimSpace(agentID)
	sessionID = strings.TrimSpace(sessionID)
	if agentID == "" {
		return nil, fmt.Errorf("agent id cannot be empty")
	}
	if sessionID == "" {
		return nil, fmt.Errorf("session id cannot be empty")
	}
	if limit <= 0 {
		return nil, nil
	}

	var records []chatMessageRecord
	if err := s.db.WithContext(ctx).
		Where("user_id = ? AND agent_id = ? AND session_id = ?", userID, agentID, sessionID).
		Order("id DESC").
		Limit(limit).
		Find(&records).Error; err != nil {
		return nil, fmt.Errorf("list recent chat messages: %w", err)
	}

	messages := make([]ChatMessage, 0, len(records))
	for i := len(records) - 1; i >= 0; i-- {
		messages = append(messages, records[i].toMessage())
	}
	return messages, nil
}

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

func modelSelectionRecordFromSelection(selection ModelSelection) modelSelectionRecord {
	return modelSelectionRecord{
		UserID:    selection.UserID,
		AgentID:   selection.AgentID,
		SessionID: selection.SessionID,
		LLMName:   selection.LLMName,
		Model:     selection.Model,
		LLMIndex:  selection.LLMIndex,
	}
}

func (r modelSelectionRecord) toSelection() ModelSelection {
	return ModelSelection{
		UserID:    r.UserID,
		AgentID:   r.AgentID,
		SessionID: r.SessionID,
		LLMName:   r.LLMName,
		Model:     r.Model,
		LLMIndex:  r.LLMIndex,
		UpdatedAt: r.UpdatedAt,
	}
}

func chatMessageRecordFromMessage(message ChatMessage) chatMessageRecord {
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

func (r chatMessageRecord) toMessage() ChatMessage {
	return ChatMessage{
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

func normalizeKey(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}
