// Package router 集中注册 Gin 路由与中间件。
//
// 所有路由都在这里统一组装，便于审阅 API 表面并避免散落式注册。
package router

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Serendipity565/gora/internal/server/handler"
	"github.com/Serendipity565/gora/internal/server/middleware"
)

// Options 控制 Gin 引擎的可选行为。
type Options struct {
	// CORS 为 true 时启用简易跨域中间件。
	CORS bool
}

// New 装配并返回一个配置好的 *gin.Engine。
//
// h 提供具体的请求处理实现；opts 控制中间件选项。
func New(h *handler.Handler, opts Options) *gin.Engine {
	engine := gin.New()
	engine.Use(gin.Logger(), gin.Recovery())
	if opts.CORS {
		engine.Use(middleware.Cors())
	}

	// 健康检查
	engine.GET("/health", handler.HandleHealth)

	api := engine.Group("/api")
	{
		api.POST("/chat", h.HandleChat)
		api.POST("/chat/:agentId", h.HandleChat)
		api.POST("/chat/tool-permission", h.HandleToolPermission)
		api.GET("/agents", h.HandleListAgents)
		api.GET("/agents/:agentId", h.HandleGetAgent)
		api.GET("/agents/:agentId/state", h.HandleGetAgent)
		api.GET("/tools", h.HandleListTools)
		api.GET("/models", h.HandleListModels)
		api.GET("/models/current", h.HandleGetCurrentModel)
		api.POST("/models/select", h.HandleSelectModel)
	}

	// 纯 API 服务：非 /api、非 /health 的请求一律 404，提示用户去前端项目。
	engine.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "not found",
			"hint":  "请确认请求地址是否正确",
		})
	})

	return engine
}
