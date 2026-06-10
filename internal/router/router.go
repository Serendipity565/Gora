// Package router 集中注册 Gin 路由与中间件。
//
// 所有路由都在这里统一组装，便于审阅 API 表面并避免散落式注册。
package router

import (
	"net/http"

	"github.com/Serendipity565/gora/internal/controller"
	"github.com/Serendipity565/gora/internal/middleware"
	"github.com/gin-gonic/gin"
)

// NewEngine 装配并返回一个配置好的 *gin.Engine。
//
// 中间件应用规则：
//   - cors / log / limit 对所有 /api 请求生效；
//   - auth (JWT) 仅对 user 模块的受保护接口生效；
//   - basicauth 当前未使用（保留依赖项以便未来挂 /metrics）。
func NewEngine(
	corsMiddleware *middleware.CorsMiddleware,
	authMiddleware *middleware.AuthMiddleware,
	basicAuthMiddleware *middleware.BasicAuthMiddleware,
	logMiddleware *middleware.LoggerMiddleware,
	limitMiddleware *middleware.LimitMiddleware,

	user controller.UserHandler,
	agent controller.AgentHandler,
	tool controller.ToolHandler,
	model controller.ModelHandler,
	chat controller.ChatHandler,
	health controller.HealthHandler,
) *gin.Engine {
	gin.ForceConsoleColor()
	r := gin.Default()

	// 全局中间件
	r.Use(corsMiddleware.MiddlewareFunc()) // 跨域中间件
	r.Use(logMiddleware.MiddlewareFunc())  // 日志中间件
	r.Use(limitMiddleware.Middleware())    // 限流中间件

	// 健康检查（不走 /api，不走鉴权）
	RegisterHealthRouter(r, health)

	api := r.Group("/api")

	RegisterUserRouter(api, user, authMiddleware.MiddlewareFunc())
	RegisterAgentRouter(api, agent)
	RegisterToolRouter(api, tool)
	RegisterModelRouter(api, model)
	RegisterChatRouter(api, chat)

	// basicAuthMiddleware 暂未挂载到任何路由（预留给未来 /metrics 等运维端点）。
	// 保留参数避免 wire 忽略依赖，使重新引入时无需改动 NewEngine 签名。
	_ = basicAuthMiddleware

	// 纯 API 服务：非 /api、非 /health 的请求一律 404，提示用户去前端项目。
	r.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "not found",
			"hint":  "请确认请求地址是否正确",
		})
	})

	return r
}
