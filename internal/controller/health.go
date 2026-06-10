package controller

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// HandleHealth 返回 200 + 简单的状态 payload，用于健康检查。
func HandleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "ok",
		"framework": "Gora",
		"time":      time.Now().UTC().Format(time.RFC3339),
	})
}
