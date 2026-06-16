package router

import (
	"github.com/Serendipity565/gora/internal/controller"
	"github.com/Serendipity565/gora/pkg/ginx"
	"github.com/gin-gonic/gin"
)

// RegisterModelRouter 把 /api/models 路由组挂到 r 上。
func RegisterModelRouter(r *gin.RouterGroup, h controller.ModelHandler, authMiddleware gin.HandlerFunc) {
	c := r.Group("/models")
	{
		c.GET("", ginx.Wrap(h.List))
		c.GET("/current", authMiddleware, ginx.WrapClaims(h.Current))
		c.POST("/select", authMiddleware, ginx.WrapClaimsAndReq(h.Select))
	}
}
