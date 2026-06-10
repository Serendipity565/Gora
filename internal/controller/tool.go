package controller

import (
	"github.com/Serendipity565/gora/api/response"
	"github.com/Serendipity565/gora/internal/server"
	"github.com/gin-gonic/gin"
)

// ToolHandler 暴露 /api/tools 路由。
type ToolHandler interface {
	List(c *gin.Context) (response.Response, error)
}

type Tool struct {
	s server.ToolService
}

func NewTool(s server.ToolService) ToolHandler {
	return &Tool{s: s}
}

// List 列出所有已注册工具，用于前端展示。
//
//	@Summary		列出工具
//	@Description	返回所有已注册工具的元信息（按 name 升序）
//	@Tags			Tool
//	@ID				listTools
//	@Produce		json
//	@Success		200	{object}	response.Response	"tools 数组"
//	@Router			/api/tools [get]
func (h *Tool) List(c *gin.Context) (response.Response, error) {
	infos := h.s.List()
	return response.Response{
		Code:    0,
		Message: "success",
		Data:    gin.H{"tools": infos},
	}, nil
}
