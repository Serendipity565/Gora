package router

import (
	"github.com/Serendipity565/gora/internal/controller"
	"github.com/Serendipity565/gora/pkg/ginx"
	"github.com/gin-gonic/gin"
)

// RegisterAgentRouter 把 /api/agents 路由组挂到 r 上。
func RegisterAgentRouter(r *gin.RouterGroup, h controller.AgentHandler) {
	c := r.Group("/agents")
	{
		c.GET("", ginx.Wrap(h.List))
		c.GET("/:agentId", ginx.Wrap(h.Get))
		c.GET("/:agentId/state", ginx.Wrap(h.Get))
	}
}
