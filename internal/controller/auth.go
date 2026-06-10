package controller

import (
	"net/http"
	"strings"

	"github.com/Serendipity565/gora/internal/middleware"
	"github.com/gin-gonic/gin"

	"github.com/Serendipity565/gora/pkg/ginx"
	"github.com/Serendipity565/gora/pkg/ijwt"
)

// AuthHandler 暴露登录 / 签发 token 的 HTTP 接口。
//
// 当前以 BasicAuth 配置中的账号作为唯一可信凭据来源 —— 与 FeedBack-Backend
// 那种基于 DB 的注册流程不同，Gora 暂时不维护用户表；后续接入用户系统时
// 把 verify(...) 换成 dao 调用即可。
//
// jwt 为 nil 时所有请求被拒（500），避免静默生成无法验证的 token。
type AuthHandler struct {
	jwt   *ijwt.JWT
	basic *middleware.BasicAuthMiddleware
}

// NewAuthHandler 构造 AuthHandler。
func NewAuthHandler(jwt *ijwt.JWT, basic *middleware.BasicAuthMiddleware) *AuthHandler {
	return &AuthHandler{jwt: jwt, basic: basic}
}

// loginRequest 登录请求体。
type loginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// loginResponse 登录响应体。
type loginResponse struct {
	Token    string `json:"token"`
	Username string `json:"username"`
}

// HandleLogin 校验用户名 / 密码后签发 JWT。
//
// 校验逻辑复用 BasicAuth 的账号映射，避免再开一份用户配置。
func (h *AuthHandler) HandleLogin(c *gin.Context) {
	if h.jwt == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"code":    http.StatusServiceUnavailable,
			"message": "JWT 未配置，登录端点不可用",
		})
		return
	}

	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    http.StatusBadRequest,
			"message": "请求体解析失败",
		})
		return
	}
	req.Username = strings.TrimSpace(req.Username)

	if h.basic == nil || !h.basic.Verify(req.Username, req.Password) {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    http.StatusUnauthorized,
			"message": "用户名或密码错误",
		})
		return
	}

	token, err := h.jwt.SignToken(ijwt.UserClaims{
		UserID:   req.Username,
		Username: req.Username,
		Role:     "user",
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    http.StatusInternalServerError,
			"message": "签发令牌失败",
		})
		return
	}

	c.JSON(http.StatusOK, loginResponse{
		Token:    token,
		Username: req.Username,
	})
}

// HandleWhoAmI 是 /api/secure 路由组的示例 controller：
// 它从 gin.Context 中读出 AuthMiddleware 写入的 UserClaims，回显给客户端。
//
// 没有 claims 时返回 401（理论上不会发生：能进入此 controller 说明已通过 Auth 中间件）。
func HandleWhoAmI(c *gin.Context) {
	claims, ok := ginx.GetClaims(c)
	if !ok || claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    http.StatusUnauthorized,
			"message": "未鉴权",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"user_id":  claims.UserID,
		"username": claims.Username,
		"role":     claims.Role,
	})
}
