package controller

import (
	"net/http"
	"strings"

	"github.com/Serendipity565/gora/api/request"
	"github.com/gin-gonic/gin"
)

// HandleListModels 列出当前配置中所有可选模型。
func (h *Handler) HandleListModels(c *gin.Context) {
	if h.modelSelector == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "model selector not configured"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"models": h.modelSelector.ListModels()})
}

// HandleGetCurrentModel 返回某会话当前使用的模型。
// session_id 通过 query 传入；为空表示默认会话。
func (h *Handler) HandleGetCurrentModel(c *gin.Context) {
	if h.modelSelector == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "model selector not configured"})
		return
	}

	sessionID := strings.TrimSpace(c.Query("session_id"))
	info, explicit, err := h.modelSelector.CurrentModel(c.Request.Context(), sessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"model":    info,
		"explicit": explicit,
	})
}

// HandleSelectModel 设置某会话使用的模型。
func (h *Handler) HandleSelectModel(c *gin.Context) {
	if h.modelSelector == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "model selector not configured"})
		return
	}

	var req request.ModelSelect
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	info, err := h.modelSelector.SelectModel(c.Request.Context(), strings.TrimSpace(req.SessionID), strings.TrimSpace(req.Selector))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"model": info})
}
