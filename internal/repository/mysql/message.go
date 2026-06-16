package mysql

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"

	"github.com/Serendipity565/gora/internal/repository/model"
)

// MessageDAO 提供 Message 表的读写能力。
type MessageDAO interface {
	Append(ctx context.Context, messages []model.Message) error
	ListBySession(ctx context.Context, sessionID string, opts ...MessageQueryOption) ([]model.Message, error)
	ListRecent(ctx context.Context, sessionID string, limit int) ([]model.Message, error)
	NextSeq(ctx context.Context, sessionID string) (uint64, error)
}

type messageDAO struct {
	db *gorm.DB
}

func NewMessageDAO(db *gorm.DB) MessageDAO {
	return &messageDAO{db: db}
}

// MessageQueryOption 列表查询条件 Option 模式。
type MessageQueryOption func(*messageQueryConfig)

type messageQueryConfig struct {
	cursor    uint64
	hasCursor bool
	limit     int
}

// AfterID 仅返回 id > cursor 的消息（用于分页）。
func AfterID(cursor uint64) MessageQueryOption {
	return func(c *messageQueryConfig) {
		c.cursor = cursor
		c.hasCursor = true
	}
}

// MessageLimit 限制返回条数，<=0 不生效。
func MessageLimit(limit int) MessageQueryOption {
	return func(c *messageQueryConfig) { c.limit = limit }
}

// Append 批量插入消息。
//
// 入参允许为空（直接 return nil），便于上层无脑调用。
func (m *messageDAO) Append(ctx context.Context, messages []model.Message) error {
	if len(messages) == 0 {
		return nil
	}
	for i := range messages {
		messages[i].SessionID = strings.TrimSpace(messages[i].SessionID)
		if messages[i].SessionID == "" {
			return errors.New("session id cannot be empty")
		}
		if messages[i].Role == "" {
			return errors.New("role cannot be empty")
		}
	}
	return m.db.WithContext(ctx).Create(&messages).Error
}

// ListBySession 按 session 拉取消息（默认 id 升序，可选 cursor / limit）。
func (m *messageDAO) ListBySession(ctx context.Context, sessionID string, opts ...MessageQueryOption) ([]model.Message, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, errors.New("session id cannot be empty")
	}

	cfg := &messageQueryConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	query := m.db.WithContext(ctx).Where("session_id = ?", sessionID)
	if cfg.hasCursor {
		query = query.Where("id > ?", cfg.cursor)
	}
	if cfg.limit > 0 {
		query = query.Limit(cfg.limit)
	}

	var messages []model.Message
	if err := query.Order("id ASC").Find(&messages).Error; err != nil {
		return nil, err
	}
	return messages, nil
}

// ListRecent 按 id 倒序读取最近 limit 条；返回时已按 id 升序排列。
func (m *messageDAO) ListRecent(ctx context.Context, sessionID string, limit int) ([]model.Message, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, errors.New("session id cannot be empty")
	}
	if limit <= 0 {
		return nil, nil
	}

	var records []model.Message
	if err := m.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Order("id DESC").
		Limit(limit).
		Find(&records).Error; err != nil {
		return nil, err
	}

	out := make([]model.Message, 0, len(records))
	for i := len(records) - 1; i >= 0; i-- {
		out = append(out, records[i])
	}
	return out, nil
}

// NextSeq 返回 session 下下一个 seq 序号（即当前 max(seq)+1）。
func (m *messageDAO) NextSeq(ctx context.Context, sessionID string) (uint64, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return 0, errors.New("session id cannot be empty")
	}

	var max uint64
	row := m.db.WithContext(ctx).Model(&model.Message{}).
		Where("session_id = ?", sessionID).
		Select("COALESCE(MAX(seq), 0)").
		Row()
	if err := row.Scan(&max); err != nil {
		return 0, err
	}
	return max + 1, nil
}
