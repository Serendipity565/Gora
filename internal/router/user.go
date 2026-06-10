package router

import (
	"github.com/Serendipity565/gora/internal/controller"
	"github.com/Serendipity565/gora/pkg/ginx"
	"github.com/gin-gonic/gin"
)

func RegisterUserRouter(r *gin.RouterGroup, u controller.UserHandler, authMiddleware gin.HandlerFunc) {
	c := r.Group("/user")
	{
		c.POST("/register", ginx.WrapReq(u.Register))
		c.POST("/login", ginx.WrapReq(u.Login))
		c.POST("/profile", authMiddleware, ginx.WrapClaimsAndReq(u.UpdateProfile))
	}
}
