package controller

import (
	"time"

	"github.com/Serendipity565/gora/api/response"
	"github.com/gin-gonic/gin"
)

// HealthHandler 暴露 /health 健康检查端点。
type HealthHandler interface {
	Check(c *gin.Context) (response.Response, error)
}

type Health struct{}

func NewHealth() HealthHandler {
	return &Health{}
}

// Check 返回简单的状态 payload，用于健康检查。
//
//	@Summary		健康检查
//	@Tags			Health
//	@ID				healthCheck
//	@Produce		json
//	@Success		200	{object}	response.Response
//	@Router			/health [get]
func (h *Health) Check(c *gin.Context) (response.Response, error) {
	return response.Response{
		Code:    0,
		Message: "success",
		Data: gin.H{
			"status":    "ok",
			"framework": "Gora",
			"time":      time.Now().UTC().Format(time.RFC3339),
		},
	}, nil
}
