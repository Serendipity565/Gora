package router

import (
	"github.com/Serendipity565/gora/internal/controller"
	"github.com/Serendipity565/gora/pkg/ginx"
	"github.com/gin-gonic/gin"
)

// RegisterHealthRouter 把 /health 健康检查挂到 r 上（不走 /api 前缀）。
func RegisterHealthRouter(r *gin.Engine, h controller.HealthHandler) {
	r.GET("/health", ginx.Wrap(h.Check))
}
