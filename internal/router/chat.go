package router

import (
	"github.com/Serendipity565/gora/internal/controller"
	"github.com/Serendipity565/gora/pkg/ginx"
	"github.com/gin-gonic/gin"
)

// RegisterChatRouter 把 /api/chat 系列路由挂到 r 上。
//
// SSE 接口走 ginx.WrapSSEReq —— 由它写好 SSE 响应头与 200 状态，再调用 controller。
// 工具授权回写不走 SSE，用普通 ginx.WrapReq。
func RegisterChatRouter(r *gin.RouterGroup, h controller.ChatHandler) {
	c := r.Group("/chat")
	{
		c.POST("", ginx.WrapSSEReq(h.Chat))
		c.POST("/:agentId", ginx.WrapSSEReq(h.Chat))
		c.POST("/tool-permission", ginx.WrapReq(h.ResolveToolPermission))
	}
}
