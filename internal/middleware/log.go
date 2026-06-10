package middleware

import (
	"time"

	"github.com/Serendipity565/gora/pkg/logger"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	appconfig "github.com/Serendipity565/gora/internal/config"
)

// LoggerMiddleware 是基于结构化 Logger 的访问日志中间件。
// 输出 JSON / 结构化字段（method/path/status/latency/client_ip/errors）；
// 支持 SkipPaths 精确匹配跳过；
type LoggerMiddleware struct {
	log       logger.Logger
	skipPaths map[string]struct{}
}

// NewLoggerMiddleware 用 logger.Logger 与 LogConfig.SkipPaths 构造日志中间件。
func NewLoggerMiddleware(log logger.Logger, cfg appconfig.LogConfig) *LoggerMiddleware {
	skips := make(map[string]struct{}, len(cfg.SkipPaths))
	for _, p := range cfg.SkipPaths {
		if p != "" {
			skips[p] = struct{}{}
		}
	}
	return &LoggerMiddleware{
		log:       log,
		skipPaths: skips,
	}
}

// MiddlewareFunc 处理响应逻辑
func (m *LoggerMiddleware) MiddlewareFunc() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		start := time.Now()
		path := ctx.Request.URL.Path
		ctx.Next() // 处理请求

		// 路径在 SkipPaths 中则跳过日志记录
		if _, skip := m.skipPaths[path]; skip {
			return
		}

		cost := time.Since(start)
		if len(ctx.Errors) > 0 {
			// 有错误记录错误日志
			m.log.Error("HTTP request error",
				zap.String("method", ctx.Request.Method),
				zap.String("path", path),
				zap.Int("status", ctx.Writer.Status()),
				zap.String("client_ip", ctx.ClientIP()),
				zap.Duration("latency", cost),
				zap.String("errors", ctx.Errors.String()),
			)
		} else {
			// 正常请求记录访问日志
			m.log.Info("HTTP request success",
				zap.String("method", ctx.Request.Method),
				zap.String("path", path),
				zap.Int("status", ctx.Writer.Status()),
				zap.String("client_ip", ctx.ClientIP()),
				zap.Duration("latency", cost),
			)
		}
	}
}
