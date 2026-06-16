package mysql

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/Serendipity565/gora/internal/repository/model"
)

// SessionDAO 提供 Session 表的读写能力。
//
// 主键由业务侧生成（字符串），便于 runner / Redis 与 MySQL 共用同一 ID。
type SessionDAO interface {
	Upsert(ctx context.Context, session *model.Session) error
	UpdateLastMessageAt(ctx context.Context, sessionID string, when time.Time) error
	UpdateLLM(ctx context.Context, sessionID, llmName string) error
	FindOne(ctx context.Context, opts ...SessionQueryOption) (*model.Session, error)
	List(ctx context.Context, userID uint64, limit, offset int) ([]model.Session, error)
}

type sessionDAO struct {
	db *gorm.DB
}

func NewSessionDAO(db *gorm.DB) SessionDAO {
	return &sessionDAO{db: db}
}

// SessionQueryOption 查询条件 Option 模式。
type SessionQueryOption func(*sessionQueryConfig)

type sessionQueryConfig struct {
	id     string
	userID uint64
}

// BySessionID 按主键 ID 精确匹配。
func BySessionID(id string) SessionQueryOption {
	return func(c *sessionQueryConfig) { c.id = strings.TrimSpace(id) }
}

// BySessionUserID 按 user_id 精确匹配（与 BySessionID 组合时表示鉴权）。
func BySessionUserID(userID uint64) SessionQueryOption {
	return func(c *sessionQueryConfig) { c.userID = userID }
}

// Upsert 创建或更新一个 Session。
//
// 写入约束：
//   - ID 必须由调用方生成；为空则返回错误。
//   - UpdatedAt 由 GORM 自动维护。
func (s *sessionDAO) Upsert(ctx context.Context, session *model.Session) error {
	if session == nil {
		return errors.New("session cannot be nil")
	}
	session.ID = strings.TrimSpace(session.ID)
	if session.ID == "" {
		return errors.New("session id cannot be empty")
	}
	if session.Status == 0 {
		session.Status = model.SessionStatusActive
	}
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"user_id", "agent_ids", "title", "summary", "llm_name", "last_message_at", "status", "updated_at"}),
	}).Create(session).Error
}

// UpdateLastMessageAt 仅更新 last_message_at；用于消息追加完成后刷新 session。
func (s *sessionDAO) UpdateLastMessageAt(ctx context.Context, sessionID string, when time.Time) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return errors.New("session id cannot be empty")
	}
	return s.db.WithContext(ctx).Model(&model.Session{}).
		Where("id = ?", sessionID).
		Updates(map[string]interface{}{"last_message_at": when}).Error
}

// UpdateLLM 更新某 session 当前选用的 LLM 名。
func (s *sessionDAO) UpdateLLM(ctx context.Context, sessionID, llmName string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return errors.New("session id cannot be empty")
	}
	return s.db.WithContext(ctx).Model(&model.Session{}).
		Where("id = ?", sessionID).
		Update("llm_name", strings.TrimSpace(llmName)).Error
}

// FindOne 根据条件查询单条 Session；记录不存在返回 (nil, nil)。
func (s *sessionDAO) FindOne(ctx context.Context, opts ...SessionQueryOption) (*model.Session, error) {
	cfg := &sessionQueryConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	query := s.db.WithContext(ctx).Model(&model.Session{})
	hasCond := false
	if cfg.id != "" {
		query = query.Where("id = ?", cfg.id)
		hasCond = true
	}
	if cfg.userID > 0 {
		query = query.Where("user_id = ?", cfg.userID)
		hasCond = true
	}
	if !hasCond {
		return nil, nil
	}

	var session model.Session
	if err := query.First(&session).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &session, nil
}

// List 列出指定用户的 session，按 last_message_at 倒序。
func (s *sessionDAO) List(ctx context.Context, userID uint64, limit, offset int) ([]model.Session, error) {
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	var sessions []model.Session
	if err := s.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("last_message_at DESC, id DESC").
		Limit(limit).
		Offset(offset).
		Find(&sessions).Error; err != nil {
		return nil, err
	}
	return sessions, nil
}
