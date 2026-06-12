package server

import (
	"context"
	"encoding/json"

	"gorm.io/datatypes"

	"github.com/Serendipity565/gora/internal/domain"
	"github.com/Serendipity565/gora/internal/errs"
	"github.com/Serendipity565/gora/internal/repository"
	"github.com/Serendipity565/gora/internal/repository/model"
)

// HistoryService 暴露会话与消息的查询/创建能力（"历史"维度）。
//
// 为什么聚合在一个 service：会话与消息天然 1:N，分两个 service 反而要在 controller / runner
// 里反复跨 service 协调；放一起既能复用事务上下文，也方便后续做"会话级聚合统计"。
type HistoryService interface {
	// ListSessions 列出某用户的会话，按 last_message_at 倒序分页。
	ListSessions(ctx context.Context, userID uint64, limit, offset int) ([]*domain.Session, error)
	// ListMessages 列出某 session 的消息（默认 id 升序），可选 cursor / limit。
	// 调用方需要确保 sessionID 与 userID 匹配（GetSession 鉴权）。
	ListMessages(ctx context.Context, userID uint64, sessionID string, cursor uint64, limit int) ([]*domain.Message, error)
	// GetSession 读取单个 session，可选附带 userID 做鉴权。
	GetSession(ctx context.Context, userID uint64, sessionID string) (*domain.Session, error)
}

type historyServiceImpl struct {
	sessionDAO repository.SessionDAO
	messageDAO repository.MessageDAO
}

func NewHistoryService(sessionDAO repository.SessionDAO, messageDAO repository.MessageDAO) HistoryService {
	return &historyServiceImpl{
		sessionDAO: sessionDAO,
		messageDAO: messageDAO,
	}
}

func (h *historyServiceImpl) ListSessions(ctx context.Context, userID uint64, limit, offset int) ([]*domain.Session, error) {
	sessions, err := h.sessionDAO.List(ctx, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	out := make([]*domain.Session, 0, len(sessions))
	for i := range sessions {
		out = append(out, toDomainSession(&sessions[i]))
	}
	return out, nil
}

func (h *historyServiceImpl) ListMessages(ctx context.Context, userID uint64, sessionID string, cursor uint64, limit int) ([]*domain.Message, error) {
	// 先校验 session 归属。
	if _, err := h.GetSession(ctx, userID, sessionID); err != nil {
		return nil, err
	}

	opts := []repository.MessageQueryOption{}
	if cursor > 0 {
		opts = append(opts, repository.AfterID(cursor))
	}
	if limit > 0 {
		opts = append(opts, repository.MessageLimit(limit))
	}

	messages, err := h.messageDAO.ListBySession(ctx, sessionID, opts...)
	if err != nil {
		return nil, err
	}

	out := make([]*domain.Message, 0, len(messages))
	for i := range messages {
		out = append(out, toDomainMessage(&messages[i]))
	}
	return out, nil
}

func (h *historyServiceImpl) GetSession(ctx context.Context, userID uint64, sessionID string) (*domain.Session, error) {
	session, err := h.sessionDAO.FindOne(ctx,
		repository.BySessionID(sessionID),
		repository.BySessionUserID(userID),
	)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, errs.ErrSessionNotFound(nil)
	}
	return toDomainSession(session), nil
}

func toDomainSession(s *model.Session) *domain.Session {
	if s == nil {
		return nil
	}
	out := &domain.Session{
		ID:            s.ID,
		UserID:        s.UserID,
		Title:         s.Title,
		Summary:       s.Summary,
		LLMName:       s.LLMName,
		LastMessageAt: s.LastMessageAt,
		Status:        uint8(s.Status),
		CreatedAt:     s.CreatedAt,
	}
	if len(s.AgentIDs) > 0 {
		out.AgentIDs = decodeAgentIDs(s.AgentIDs)
	}
	return out
}

func toDomainMessage(m *model.Message) *domain.Message {
	if m == nil {
		return nil
	}
	return &domain.Message{
		ID:        m.ID,
		SessionID: m.SessionID,
		Seq:       m.Seq,
		Role:      string(m.Role),
		Content:   m.Content,
		LLMName:   m.LLMName,
		Model:     m.Model,
		CreatedAt: m.CreatedAt,
	}
}

// decodeAgentIDs 把 JSON 列存储的 agent_ids 反序列化为 []string；
// 解析失败时返回 nil（不阻断列表展示）。
func decodeAgentIDs(raw datatypes.JSON) []string {
	if len(raw) == 0 {
		return nil
	}
	var ids []string
	if err := json.Unmarshal(raw, &ids); err != nil {
		return nil
	}
	return ids
}
