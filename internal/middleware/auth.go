package middleware

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Serendipity565/gora/api/response"
	"github.com/Serendipity565/gora/pkg/ginx"
	"github.com/Serendipity565/gora/pkg/ijwt"
	"github.com/gin-gonic/gin"
)

// AuthMiddleware 通过 Authorization: Bearer <token> 鉴权，并把解析后的 UserClaims
// 通过 GetClaims 读取。
type AuthMiddleware struct {
	jwt *ijwt.JWT
}

// NewAuthMiddleware 构造 AuthMiddleware
func NewAuthMiddleware(jwt *ijwt.JWT) *AuthMiddleware {
	return &AuthMiddleware{
		jwt: jwt,
	}
}

func (m *AuthMiddleware) MiddlewareFunc() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		// 从请求中提取并解析 Token
		authCode := ctx.GetHeader("Authorization")
		if authCode == "" {
			ctx.Error(errors.New("认证头部缺失"))
			ctx.JSON(http.StatusUnauthorized, response.Response{
				Code:    http.StatusUnauthorized,
				Message: "认证头部缺失",
				Data:    nil,
			})
			return
		}
		// Bearer Token 处理
		segs := strings.Split(authCode, " ")
		if len(segs) != 2 || segs[0] != "Bearer" {
			ctx.Error(errors.New("请求头格式错误"))
			ctx.JSON(http.StatusUnauthorized, response.Response{
				Code:    http.StatusUnauthorized,
				Message: "认证头格式错误",
				Data:    nil,
			})
			return
		}

		uc, err := m.jwt.ParseToken(segs[1])
		if err != nil {
			ctx.Error(err)
			ctx.JSON(http.StatusUnauthorized, response.Response{
				Code:    http.StatusUnauthorized,
				Message: "无效或过期的身份令牌",
				Data:    nil,
			})
			return
		}

		ginx.SetClaims(ctx, uc)

		// 继续处理请求
		ctx.Next()
	}
}
