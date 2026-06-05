// Package middleware 收纳所有 Gin 中间件实现。
package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// Cors 是一个简单的跨域中间件：
//   - 默认 Access-Control-Allow-Origin 允许 Origin 头携带的源；
//   - 支持常见 Method / Header；
//   - OPTIONS 直接 204 返回。
func Cors() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin == "" {
			origin = "*"
		}
		c.Header("Access-Control-Allow-Origin", origin)
		c.Header("Vary", "Origin")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", strings.Join([]string{
			"Content-Type", "Authorization", "Accept", "X-Requested-With",
		}, ", "))
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Max-Age", "600")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
