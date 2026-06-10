package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Serendipity565/gora/api/request"
	"github.com/Serendipity565/gora/api/response"
	"github.com/Serendipity565/gora/internal/agent/core"
	"github.com/Serendipity565/gora/internal/errs"
	"github.com/Serendipity565/gora/internal/server"
)

// 默认的单次对话超时；通过 ChatOption 覆盖。
const defaultChatTimeout = 5 * time.Minute

// ChatHandler 暴露 /api/chat 系列路由：SSE 流式对话 + 工具授权回写。
//
// 两类接口绑定在同一个 Handler 上：它们共享 chat 会话状态（permission gate
// 由 chat 流注册请求、由 ResolveToolPermission 写回决定），合并管理可避免
// 双写一个 service handle 又分给两个 handler。
type ChatHandler interface {
	// Chat 处理一次 SSE 流式对话；与 ginx.WrapSSEReq 配套使用。
	Chat(c *gin.Context, req request.Chat) error
	// ResolveToolPermission 由前端在弹窗中得到用户决定后调用，
	// 把决定写回正在等待的 Agent goroutine。
	ResolveToolPermission(c *gin.Context, req request.ToolPermission) (response.Response, error)
}

// ChatOption 配置 Chat 实现的可选项。
type ChatOption func(*Chat)

// WithChatTimeout 设置单次对话的最大时长；不传或传 <=0 时使用 defaultChatTimeout。
func WithChatTimeout(d time.Duration) ChatOption {
	return func(h *Chat) {
		if d > 0 {
			h.chatTimeout = d
		}
	}
}

type Chat struct {
	chatSvc       server.ChatService
	permissionSvc server.PermissionService
	chatTimeout   time.Duration
}

func NewChat(chatSvc server.ChatService, permissionSvc server.PermissionService, opts ...ChatOption) ChatHandler {
	c := &Chat{
		chatSvc:       chatSvc,
		permissionSvc: permissionSvc,
		chatTimeout:   defaultChatTimeout,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Chat 处理一次 SSE 流式对话。
//
// controller 只做：参数解析 → 选超时 → 把 SSE 流交给 server。
// 真正的"选 agent / 流式编排 / 终态控制"由 server.ChatService 负责。
//
// 注意：该方法假设上层 ginx.WrapSSEReq 已经写好 SSE 响应头并 Status(200)。
//
//	@Summary		SSE 流式对话
//	@Tags			Chat
//	@ID				chat
//	@Accept			json
//	@Produce		text/event-stream
//	@Param			agentId	path	string			false	"指定 Agent ID"
//	@Param			request	body	request.Chat	true	"对话请求"
//	@Success		200
//	@Failure		400	{object}	response.Response
//	@Failure		404	{object}	response.Response	"agent 不存在"
//	@Router			/api/chat [post]
//	@Router			/api/chat/{agentId} [post]
func (h *Chat) Chat(c *gin.Context, req request.Chat) error {
	chatReq := server.ChatRequest{
		Message:       req.Message,
		AgentID:       resolveRequestedAgentID(c, req.AgentID),
		SessionID:     strings.TrimSpace(req.SessionID),
		DisabledTools: req.DisabledTools,
	}

	sink, err := newGinSSESink(c.Writer)
	if err != nil {
		return errs.InternalServerError(err)
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), h.chatTimeout)
	defer cancel()

	if err := h.chatSvc.Stream(ctx, chatReq, sink); err != nil {
		var notFound server.ErrAgentNotFound
		if errors.As(err, &notFound) {
			// SSE 头已经发送，无法再以 JSON 形式回写错误码；
			// 退化为发送一条 error SSE 事件 + 终止流。
			_ = sink.WriteError(notFound.Error())
			_ = sink.WriteDone()
			return nil
		}
		// 其它错误通常意味着客户端断开，已经无法再写 JSON。
		return nil
	}
	return nil
}

// ResolveToolPermission 由前端在弹窗中得到用户决定后调用，
// 把决定写回正在等待的 Agent goroutine。
//
//	@Summary		回写工具授权决定
//	@Tags			Chat
//	@ID				resolveToolPermission
//	@Accept			json
//	@Produce		json
//	@Param			request	body		request.ToolPermission	true	"授权决定"
//	@Success		200		{object}	response.Response
//	@Failure		404		{object}	response.Response	"request_id 已过期或不存在"
//	@Router			/api/chat/tool-permission [post]
func (h *Chat) ResolveToolPermission(c *gin.Context, req request.ToolPermission) (response.Response, error) {
	if h.permissionSvc == nil {
		return response.Response{}, errs.InternalServerError(nil)
	}

	resolved := h.permissionSvc.Resolve(req.RequestID, core.ToolPermissionDecision{
		Approved: req.Approve,
		Remember: req.Remember,
	})
	if !resolved {
		// request_id 已过期 / 不存在（典型情况：用户点击太晚或 Agent 已被取消）。
		return response.Response{}, errs.ErrPermissionRequestNotFound(nil)
	}
	return response.Response{
		Code:    0,
		Message: "success",
		Data:    gin.H{"resolved": true},
	}, nil
}

// resolveRequestedAgentID 优先取 path param；其次取 body 字段；都为空返回空串。
func resolveRequestedAgentID(c *gin.Context, bodyAgentID string) string {
	if agentID := strings.TrimSpace(c.Param("agentId")); agentID != "" {
		return agentID
	}
	if agentID := strings.TrimSpace(bodyAgentID); agentID != "" {
		return agentID
	}
	return ""
}

// ---------------------------------------------------------------------------
// ginSSESink — server.EventSink 的 gin.ResponseWriter 实现。
// 不再依赖独立的 sse 包，所有 SSE 写入都走 gin 原生 Writer + Flush。
// ---------------------------------------------------------------------------

type sseEventPayload struct {
	Type      string         `json:"type"`
	AgentID   string         `json:"agent_id,omitempty"`
	Content   string         `json:"content,omitempty"`
	Timestamp string         `json:"timestamp,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type ginSSESink struct {
	w       http.ResponseWriter
	flusher http.Flusher
}

// newGinSSESink 把 gin.ResponseWriter 包装成 server.EventSink。
//
// 调用方应保证 http.ResponseWriter 实现了 http.Flusher，否则返回错误。
// 响应头与状态码由上层 ginx.WrapSSEReq 写好；这里只负责写 body。
func newGinSSESink(w http.ResponseWriter) (*ginSSESink, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("streaming not supported")
	}
	flusher.Flush()
	return &ginSSESink{w: w, flusher: flusher}, nil
}

// WriteEvent 把单个 Agent 事件序列化为 SSE 帧。
func (s *ginSSESink) WriteEvent(event core.Event) error {
	data, err := json.Marshal(sseEventPayload{
		Type:      event.Type.String(),
		AgentID:   event.AgentID,
		Content:   event.Content,
		Timestamp: event.Timestamp.Format("2006-01-02T15:04:05.000000000Z07:00"),
		Metadata:  event.Metadata,
	})
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", event.Type.String(), data); err != nil {
		return err
	}
	s.flusher.Flush()
	return nil
}

// WriteDone 发送独立的 done 事件，让前端关闭流。
func (s *ginSSESink) WriteDone() error {
	data, err := json.Marshal(sseEventPayload{Type: core.EventDone.String()})
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(s.w, "event: done\ndata: %s\n\n", data); err != nil {
		return err
	}
	s.flusher.Flush()
	return nil
}

// WriteError 把错误以 SSE 错误事件的形式发送给前端。
func (s *ginSSESink) WriteError(message string) error {
	event := core.NewErrorEvent("system", fmt.Errorf("%s", message))
	return s.WriteEvent(event)
}
