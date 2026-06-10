// Package middleware 收纳所有 Gin 中间件实现。
package middleware

import (
	"time"

	"github.com/Serendipity565/gora/internal/config"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

type CorsMiddleware struct {
	allowedOrigins []string
	allowedMethods []string
	allowedHeaders []string
}

func NewCorsMiddleware(cfg *config.CorsConfig) *CorsMiddleware {
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
