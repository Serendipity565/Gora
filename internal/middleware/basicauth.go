package middleware

import (
	"github.com/Serendipity565/gora/configs"
	"github.com/gin-gonic/gin"
)

// BasicAuthMiddleware 把 config.Middleware.BasicAuth 列表转成 gin.BasicAuth 凭据。
type BasicAuthMiddleware struct {
	accounts gin.Accounts
}

// NewBasicAuthMiddleware 从配置构造 BasicAuthMiddleware；空列表得到一个透传中间件。
func NewBasicAuthMiddleware(basicUsers []configs.BasicAuthAccount) *BasicAuthMiddleware {
	accounts := gin.Accounts{}
	for _, u := range basicUsers {
		accounts[u.Username] = u.Password
	}

	return &BasicAuthMiddleware{
		accounts: accounts,
	}
}

// MiddlewareFunc 返回 gin.BasicAuth 中间件；
// 未配置任何用户时退化为 c.Next() 透传。
func (m *BasicAuthMiddleware) MiddlewareFunc() gin.HandlerFunc {
	if len(m.accounts) == 0 {
		return func(c *gin.Context) { c.Next() }
	}
	return gin.BasicAuth(m.accounts)
}
