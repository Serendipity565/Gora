package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Serendipity565/gora/internal/server/api/v1/request"
	"github.com/Serendipity565/gora/internal/server/service"
	"github.com/Serendipity565/gora/internal/server/sse"
)

// HandleChat 处理一次 SSE 流式对话。
//
// handler 只做：参数解析 → 写头 → 把 SSE 流交给 service。
// 真正的"选 agent / 流式编排 / 终态控制"由 service.ChatService 负责。
func (h *Handler) HandleChat(c *gin.Context) {
	var req request.Chat
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	chatReq := service.ChatRequest{
		Message:       req.Message,
		AgentID:       resolveRequestedAgentID(c, req.AgentID),
		SessionID:     strings.TrimSpace(req.SessionID),
		DisabledTools: req.DisabledTools,
	}

	sw, err := sse.NewSSEWriter(c.Writer)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), h.chatTimeout)
	defer cancel()

	if err := h.chatService.Stream(ctx, chatReq, sw); err != nil {
		var notFound service.ErrAgentNotFound
		if errors.As(err, &notFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": notFound.Error()})
			return
		}
		// 其它错误通常意味着客户端断开，已经无法再写 JSON。
	}
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
