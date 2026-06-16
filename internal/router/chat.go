package router

import (
	"github.com/Serendipity565/gora/internal/controller"
	"github.com/Serendipity565/gora/pkg/ginx"
	"github.com/gin-gonic/gin"
)

// RegisterChatRouter 把 /api/chat 系列路由挂到 r 上。
//
// SSE 接口走 ginx.WrapSSEClaimsAndReq —— 提取 JWT claims 并写好 SSE 响应头与 200 状态，再调用 controller。
// 工具授权回写不走 SSE，用 ginx.WrapClaimsAndReq。
func RegisterChatRouter(r *gin.RouterGroup, h controller.ChatHandler, authMiddleware gin.HandlerFunc) {
	c := r.Group("/chat")
	{
		c.POST("", authMiddleware, ginx.WrapSSEClaimsAndReq(h.Chat))
		c.POST("/:agentId", authMiddleware, ginx.WrapSSEClaimsAndReq(h.Chat))
		c.POST("/tool-permission", authMiddleware, ginx.WrapClaimsAndReq(h.ResolveToolPermission))
	}
}
