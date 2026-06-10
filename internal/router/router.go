// Package router 集中注册 Gin 路由与中间件。
//
// 所有路由都在这里统一组装，便于审阅 API 表面并避免散落式注册。
package router

import (
	"net/http"

	"github.com/Serendipity565/gora/internal/controller"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/Serendipity565/gora/internal/middleware"
)

func NewEngine(
	corsMiddleware *middleware.CorsMiddleware,
	authMiddleware *middleware.AuthMiddleware,
	basicAuthMiddleware *middleware.BasicAuthMiddleware,
	logMiddleware *middleware.LoggerMiddleware,
	limitMiddleware *middleware.LimitMiddleware,
	u controller.UserHandler,
) *gin.Engine {
	gin.ForceConsoleColor()
	r := gin.Default()

	// 全局中间件
	r.Use(corsMiddleware.MiddlewareFunc()) // 跨域中间件
	r.Use(logMiddleware.MiddlewareFunc())  // 日志中间件
	r.Use(limitMiddleware.Middleware())    // 限流中间件

	api := r.Group("/api")

	RegisterUserRouter(api, u, authMiddleware.MiddlewareFunc())
	return r

}

// Options 控制 Gin 引擎的可选行为。
type Options struct {
	// CORS 为 true 时启用简易跨域中间件。
	CORS bool
}

// New 装配并返回一个配置好的 *gin.Engine。
//
// h            — 请求处理实现。
// authHandler  — /api/auth/* 登录接口；nil 时跳过该路由组。
// mw           — middleware Bundle；nil 时仅启用 gin.Logger / gin.Recovery / 可选 CORS。
// opts         — 中间件可选开关。
//
// 中间件应用规则（参考 muxi-Infra/FeedBack-Backend）：
//   - log / prometheus / cors 对所有请求生效（log 内部按 LogConfig.SkipPaths 过滤）；
//   - limit 对 /api 全组生效；
//   - auth (JWT) 仅对 /api/secure/* 生效；
//   - basicauth 仅对 /metrics 生效（运维端点）。
func New(h *handler.Handler, authHandler *handler.AuthHandler, mw *middleware.Bundle, opts Options) *gin.Engine {
	engine := gin.New()
	engine.Use(gin.Recovery())

	if mw != nil {
		// 结构化日志取代 gin.Logger()。
		engine.Use(mw.Log.Handle())
		engine.Use(mw.Prometheus.Handle())
	} else {
		engine.Use(gin.Logger())
	}
	if opts.CORS {
		if mw != nil {
			engine.Use(mw.Cors)
		} else {
			engine.Use(middleware.Cors())
		}
	}

	// 健康检查（不走鉴权 / 限流）。
	engine.GET("/health", handler.HandleHealth)

	// /metrics：basicauth 保护的运维端点。
	if mw != nil {
		engine.GET("/metrics", mw.BasicAuth.Handle(), gin.WrapH(promhttp.HandlerFor(
			mw.Prometheus.Registry(), promhttp.HandlerOpts{},
		)))
	}

	// /api 路由组：默认套上限流。
	api := engine.Group("/api")
	if mw != nil {
		api.Use(mw.Limit.Handle())
	}
	{
		// 登录接口（无鉴权 —— 鉴权前的入口）
		if authHandler != nil {
			api.POST("/auth/login", authHandler.HandleLogin)
		}

		// 业务接口（保留原有 API 表面，向后兼容）
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

	// /api/secure：JWT 保护的子路由组。预留给未来"用户私有数据"类接口。
	if mw != nil {
		secure := api.Group("/secure")
		secure.Use(mw.Auth.Handle())
		// 示例：访问者必须携带有效 JWT，controller 通过 ginx.GetClaims 取用户信息。
		secure.GET("/whoami", handler.HandleWhoAmI)
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
