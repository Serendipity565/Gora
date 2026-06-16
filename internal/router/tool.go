package router

import (
	"github.com/Serendipity565/gora/internal/controller"
	"github.com/Serendipity565/gora/pkg/ginx"
	"github.com/gin-gonic/gin"
)

// RegisterToolRouter 把 /api/tools 路由挂到 r 上。
func RegisterToolRouter(r *gin.RouterGroup, h controller.ToolHandler) {
	r.GET("/tools", ginx.Wrap(h.List))
}
