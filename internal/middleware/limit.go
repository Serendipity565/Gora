package middleware

import (
	_ "embed"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Serendipity565/gora/api/response"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"

	appconfig "github.com/Serendipity565/gora/internal/config"
)

//go:embed scripts/limiter.lua
var limiterScriptSource string

// LimitMiddleware 是基于 Redis + Lua 的令牌桶限流。
type LimitMiddleware struct {
	capacity     int // 容量
	fillInterval int // 每秒补充令牌的次数
	quantum      int // 每次发放令牌数量
	client       redis.Cmdable
	script       *redis.Script
}

// NewLimitMiddleware 构造限流中间件。
func NewLimitMiddleware(cfg appconfig.LimiterConfig, client *redis.Client) *LimitMiddleware {
	return &LimitMiddleware{
		capacity:     cfg.Capacity,
		fillInterval: cfg.FillInterval,
		quantum:      cfg.Quantum,
		client:       client,
		script:       redis.NewScript(limiterScriptSource),
	}
}

// Middleware 返回 gin.HandlerFunc。
func (m *LimitMiddleware) Middleware() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		prefix := "gora-limit" + strings.ReplaceAll(ctx.FullPath(), ":", "_")
		if prefix == "gora-limit:" {
			// 未注册路由使用统一前缀,避免浪费 redis 资源
			prefix = "gora-limit_unregistered"
		}
		prefix = prefix + "_"

		availableKey := prefix + "tokens"
		latestKey := prefix + "ts"
		now := time.Now().UnixMilli()
		res, err := m.script.Run(
			ctx.Request.Context(),
			m.client,
			[]string{availableKey, latestKey},
			m.capacity,     // ARGV[1]
			m.quantum,      // ARGV[2]
			m.fillInterval, // ARGV[3] 每秒补充几次
			now,            // ARGV[4] 毫秒
			1,              // ARGV[5]
		).Int()
		if err != nil {
			ctx.Error(fmt.Errorf("限流器执行错误: %v", err))
			ctx.JSON(http.StatusInternalServerError, response.Response{
				Code:    http.StatusInternalServerError,
				Message: "限流器内部错误",
				Data:    nil,
			})
			return
		}
		if res == 0 {
			ctx.Error(errors.New("请求过于频繁，请稍后再试"))
			ctx.JSON(http.StatusTooManyRequests, response.Response{
				Code:    http.StatusTooManyRequests,
				Message: "请求过于频繁，请稍后再试",
				Data:    nil,
			})
			return
		}
		ctx.Next()
	}
}
