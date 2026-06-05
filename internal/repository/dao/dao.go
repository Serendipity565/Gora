package dao

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/Serendipity565/gora/internal/repository/model"
)

// LocalUserID 是缺省 user_id，用于 CLI 等不区分用户的场景。
const LocalUserID = "local"

// ModelSelectionDAO 暴露 (user, agent, session) → 当前模型 的读写能力。
type ModelSelectionDAO interface {
	Get(ctx context.Context, userID, agentID, sessionID string) (model.ModelSelection, bool, error)
	Save(ctx context.Context, selection model.ModelSelection) error
	Close() error
}

// ChatHistoryDAO 暴露聊天历史的追加 / 查询能力。
type ChatHistoryDAO interface {
	AppendMessages(ctx context.Context, messages []model.ChatMessage) error
	ListRecentMessages(ctx context.Context, userID, agentID, sessionID string, limit int) ([]model.ChatMessage, error)
}

// DatabaseStore 是同时承担「模型选择 + 聊天历史」的复合接口。
type DatabaseStore interface {
	ModelSelectionDAO
	ChatHistoryDAO
}

// NoopDatabaseStore 在未配置 MySQL 时使用，所有操作均为无副作用空实现。
type NoopDatabaseStore struct{}

func (NoopDatabaseStore) Get(context.Context, string, string, string) (model.ModelSelection, bool, error) {
	return model.ModelSelection{}, false, nil
}

func (NoopDatabaseStore) Save(context.Context, model.ModelSelection) error { return nil }

func (NoopDatabaseStore) Close() error { return nil }

func (NoopDatabaseStore) AppendMessages(context.Context, []model.ChatMessage) error { return nil }

func (NoopDatabaseStore) ListRecentMessages(context.Context, string, string, string, int) ([]model.ChatMessage, error) {
	return nil, nil
}

// OpenDatabaseStore 按 DSN 打开 MySQL；DSN 为空时退化为 NoopDatabaseStore。
func OpenDatabaseStore(ctx context.Context, databaseURL string) (DatabaseStore, error) {
	databaseURL = strings.TrimSpace(databaseURL)
	if databaseURL == "" {
		return NoopDatabaseStore{}, nil
	}
	return NewGormDatabaseStore(ctx, databaseURL)
}

// GormDatabaseStore 是基于 GORM 的复合实现，同时满足 ModelSelectionDAO + ChatHistoryDAO。
type GormDatabaseStore struct {
	db *gorm.DB
}

// NewGormDatabaseStore 建立 MySQL 连接并自动迁移表结构。
func NewGormDatabaseStore(ctx context.Context, databaseURL string) (*GormDatabaseStore, error) {
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

	store := &GormDatabaseStore{db: db}
	if err := store.ensureSchema(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	return store, nil
}

func (s *GormDatabaseStore) ensureSchema(ctx context.Context) error {
	if err := s.db.WithContext(ctx).AutoMigrate(&modelSelectionRecord{}, &chatMessageRecord{}); err != nil {
		return fmt.Errorf("ensure storage schema: %w", err)
	}
	return nil
}

func (s *GormDatabaseStore) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// Get 读取模型选择。
func (s *GormDatabaseStore) Get(ctx context.Context, userID, agentID, sessionID string) (model.ModelSelection, bool, error) {
	selection := model.ModelSelection{
		UserID:    normalizeKey(userID, LocalUserID),
		AgentID:   strings.TrimSpace(agentID),
		SessionID: strings.TrimSpace(sessionID),
	}
	if selection.AgentID == "" {
		return model.ModelSelection{}, false, fmt.Errorf("agent id cannot be empty")
	}
	if selection.SessionID == "" {
		return model.ModelSelection{}, false, fmt.Errorf("session id cannot be empty")
	}

	var record modelSelectionRecord
	err := s.db.WithContext(ctx).
		Where("user_id = ? AND agent_id = ? AND session_id = ?", selection.UserID, selection.AgentID, selection.SessionID).
		First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.ModelSelection{}, false, nil
	}
	if err != nil {
		return model.ModelSelection{}, false, fmt.Errorf("get model selection: %w", err)
	}
	return record.toSelection(), true, nil
}

// Save upsert 模型选择。
func (s *GormDatabaseStore) Save(ctx context.Context, selection model.ModelSelection) error {
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

// AppendMessages 批量插入聊天历史。
func (s *GormDatabaseStore) AppendMessages(ctx context.Context, messages []model.ChatMessage) error {
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

// ListRecentMessages 按时间倒序读取最新 limit 条。
func (s *GormDatabaseStore) ListRecentMessages(ctx context.Context, userID, agentID, sessionID string, limit int) ([]model.ChatMessage, error) {
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

	messages := make([]model.ChatMessage, 0, len(records))
	for i := len(records) - 1; i >= 0; i-- {
		messages = append(messages, records[i].toMessage())
	}
	return messages, nil
}

func normalizeKey(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}
