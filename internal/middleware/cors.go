// Package middleware 收纳所有 Gin 中间件实现。
package middleware

import (
	"time"

	"github.com/Serendipity565/gora/configs"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// CorsMiddleware 处理跨域请求，基于 gin-contrib/cors 实现。
type CorsMiddleware struct {
	allowedOrigins []string
	allowedMethods []string
	allowedHeaders []string
}

// NewCorsMiddleware 根据配置创建 CORS 中间件。
func NewCorsMiddleware(cfg configs.CorsConfig) *CorsMiddleware {
	return &CorsMiddleware{
		allowedOrigins: cfg.AllowedOrigins,
		allowedMethods: cfg.AllowedMethods,
		allowedHeaders: cfg.AllowedHeaders,
	}
}

func (m *CorsMiddleware) MiddlewareFunc() gin.HandlerFunc {
	return cors.New(cors.Config{
		AllowOrigins: m.allowedOrigins,
		AllowMethods: m.allowedMethods,
		AllowHeaders: m.allowedHeaders,
		MaxAge:       2 * time.Hour,
	})
}
