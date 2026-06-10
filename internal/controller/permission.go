package controller

import (
	"net/http"

	"github.com/Serendipity565/gora/api/request"
	"github.com/gin-gonic/gin"

	"github.com/Serendipity565/gora/internal/agent/core"
)

// HandleToolPermission 由前端在弹窗中得到用户决定后调用，
// 把决定写回正在等待的 Agent goroutine。
func (h *Handler) HandleToolPermission(c *gin.Context) {
	var req request.ToolPermission
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if h.permissionGate == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "permission gate not configured"})
		return
	}

	resolved := h.permissionGate.Resolve(req.RequestID, core.ToolPermissionDecision{
		Approved: req.Approve,
		Remember: req.Remember,
	})
	if !resolved {
		// request_id 已过期 / 不存在（典型情况：用户点击太晚或 Agent 已被取消）。
		c.JSON(http.StatusNotFound, gin.H{"error": "permission request not found or already resolved"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"resolved": true})
}
