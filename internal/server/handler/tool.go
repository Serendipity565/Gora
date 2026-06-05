package handler

import (
	"net/http"
	"sort"

	"github.com/gin-gonic/gin"

	"github.com/Serendipity565/gora/internal/server/api/v1/response"
)

// HandleListTools 列出所有已注册工具，用于前端展示。
func (h *Handler) HandleListTools(c *gin.Context) {
	if h.registry == nil {
		c.JSON(http.StatusOK, gin.H{"tools": []response.ToolInfo{}})
		return
	}

	tools := h.registry.List()
	infos := make([]response.ToolInfo, 0, len(tools))
	for _, t := range tools {
		infos = append(infos, response.ToolInfo{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  t.Parameters(),
		})
	}
	sort.Slice(infos, func(i, j int) bool {
		return infos[i].Name < infos[j].Name
	})
	c.JSON(http.StatusOK, gin.H{"tools": infos})
}
