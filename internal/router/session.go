package router

import (
	"github.com/gin-gonic/gin"

	"github.com/Serendipity565/gora/internal/controller"
	"github.com/Serendipity565/gora/pkg/ginx"
)

// RegisterSessionRouter 把 /api/sessions 系列路由挂到 r 上。
//
// 全部接口需要登录：列出当前用户的会话；按 sessionId 拉取消息（service 层校验归属）。
func RegisterSessionRouter(r *gin.RouterGroup, h controller.HistoryHandler, authMiddleware gin.HandlerFunc) {
	s := r.Group("/sessions", authMiddleware)
	{
		s.GET("", ginx.WrapClaims(h.ListSessions))
		s.GET("/:sessionId/messages", ginx.WrapClaimsAndReq(h.ListMessages))
	}
}
