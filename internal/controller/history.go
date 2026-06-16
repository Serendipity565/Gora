package controller

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Serendipity565/gora/api/request"
	"github.com/Serendipity565/gora/api/response"
	"github.com/Serendipity565/gora/internal/domain"
	"github.com/Serendipity565/gora/internal/errs"
	"github.com/Serendipity565/gora/internal/server"
	"github.com/Serendipity565/gora/pkg/ijwt"
)

// HistoryHandler 暴露 /api/sessions 系列路由：会话列表 + 单会话消息列表。
//
// 历史能力天然 1:N（session → messages），合并为一个 handler 比拆成两个更易维护。
type HistoryHandler interface {
	ListSessions(c *gin.Context, uc ijwt.UserClaims) (response.Response, error)
	ListMessages(c *gin.Context, req request.ListMessages, uc ijwt.UserClaims) (response.Response, error)
}

type History struct {
	s server.HistoryService
}

func NewHistory(s server.HistoryService) HistoryHandler {
	return &History{s: s}
}

// ListSessions 列出当前用户的会话。
//
//	@Summary		列出会话
//	@Description	返回当前登录用户的会话，按 last_message_at 倒序
//	@Tags			Session
//	@ID				listSessions
//	@Produce		json
//	@Param			Authorization	header		string	true	"Bearer Token"
//	@Param			limit			query		int		false	"分页大小，默认 20"
//	@Param			offset			query		int		false	"偏移，默认 0"
//	@Success		200				{object}	response.Response{data=response.SessionsResp}
//	@Failure		401				{object}	response.Response	"未登录或 token 无效"
//	@Failure		500				{object}	response.Response	"服务器错误"
//	@Router			/api/sessions [get]
func (h *History) ListSessions(c *gin.Context, uc ijwt.UserClaims) (response.Response, error) {
	userID, err := parseUserID(uc)
	if err != nil {
		return response.Response{}, errs.ErrUserNotFound(err)
	}

	limit, offset := parseListSessionsQuery(c)
	sessions, err := h.s.ListSessions(c.Request.Context(), userID, limit, offset)
	if err != nil {
		return response.Response{}, errs.InternalServerError(err)
	}

	items := make([]response.SessionItem, 0, len(sessions))
	for _, s := range sessions {
		items = append(items, toSessionItem(s))
	}
	return response.Response{
		Code:    0,
		Message: "success",
		Data:    response.SessionsResp{Sessions: items},
	}, nil
}

// ListMessages 列出指定 session 的消息。
//
//	@Summary		列出消息
//	@Description	按 session_id 列出该会话的消息（id 升序，可选 cursor / limit）
//	@Tags			Session
//	@ID				listMessages
//	@Produce		json
//	@Param			Authorization	header		string	true	"Bearer Token"
//	@Param			sessionId		path		string	true	"Session ID"
//	@Param			cursor			query		uint64	false	"返回 id > cursor 的消息"
//	@Param			limit			query		int		false	"分页大小"
//	@Success		200				{object}	response.Response{data=response.MessagesResp}
//	@Failure		401				{object}	response.Response	"未登录或 token 无效"
//	@Failure		403				{object}	response.Response	"无权访问该会话"
//	@Failure		404				{object}	response.Response	"会话不存在"
//	@Router			/api/sessions/{sessionId}/messages [get]
func (h *History) ListMessages(c *gin.Context, req request.ListMessages, uc ijwt.UserClaims) (response.Response, error) {
	userID, err := parseUserID(uc)
	if err != nil {
		return response.Response{}, errs.ErrUserNotFound(err)
	}

	sessionID := strings.TrimSpace(c.Param("sessionId"))
	if sessionID == "" {
		return response.Response{}, errs.ErrSessionNotFound(nil)
	}

	messages, err := h.s.ListMessages(c.Request.Context(), userID, sessionID, req.Cursor, req.Limit)
	if err != nil {
		// service 已经把"会话不存在/越权"包成 errorx，这里直接透传。
		return response.Response{}, err
	}

	items := make([]response.MessageItem, 0, len(messages))
	for _, m := range messages {
		items = append(items, toMessageItem(m))
	}
	return response.Response{
		Code:    0,
		Message: "success",
		Data:    response.MessagesResp{Messages: items},
	}, nil
}

func parseListSessionsQuery(c *gin.Context) (limit, offset int) {
	limit, _ = strconv.Atoi(c.Query("limit"))
	offset, _ = strconv.Atoi(c.Query("offset"))
	return
}

func toSessionItem(s *domain.Session) response.SessionItem {
	if s == nil {
		return response.SessionItem{}
	}
	return response.SessionItem{
		ID:            s.ID,
		AgentIDs:      s.AgentIDs,
		Title:         s.Title,
		Summary:       s.Summary,
		LLMName:       s.LLMName,
		LastMessageAt: s.LastMessageAt.Format("2006-01-02T15:04:05.000Z07:00"),
		Status:        s.Status,
		CreatedAt:     s.CreatedAt.Format("2006-01-02T15:04:05.000Z07:00"),
	}
}

func toMessageItem(m *domain.Message) response.MessageItem {
	if m == nil {
		return response.MessageItem{}
	}
	return response.MessageItem{
		ID:        m.ID,
		SessionID: m.SessionID,
		Seq:       m.Seq,
		Role:      m.Role,
		Content:   m.Content,
		ToolCalls: m.ToolCalls,
		LLMName:   m.LLMName,
		Model:     m.Model,
		CreatedAt: m.CreatedAt.Format("2006-01-02T15:04:05.000Z07:00"),
	}
}
