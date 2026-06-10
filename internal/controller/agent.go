package controller

import (
	"github.com/Serendipity565/gora/api/response"
	"github.com/Serendipity565/gora/internal/errs"
	"github.com/Serendipity565/gora/internal/server"
	"github.com/gin-gonic/gin"
)

// AgentHandler 暴露 /api/agents 路由组。
type AgentHandler interface {
	List(c *gin.Context) (response.Response, error)
	Get(c *gin.Context) (response.Response, error)
}

type Agent struct {
	s server.AgentService
}

func NewAgent(s server.AgentService) AgentHandler {
	return &Agent{s: s}
}

// List 列出所有可用 Agent 元信息。
//
//	@Summary		列出 Agent
//	@Description	返回所有已注册 Agent 的元信息（按 ID 升序）
//	@Tags			Agent
//	@ID				listAgents
//	@Produce		json
//	@Success		200	{object}	response.Response	"agents 数组"
//	@Router			/api/agents [get]
func (h *Agent) List(c *gin.Context) (response.Response, error) {
	infos := h.s.List()
	return response.Response{
		Code:    0,
		Message: "success",
		Data:    gin.H{"agents": infos},
	}, nil
}

// Get 查询单个 Agent 状态。
//
//	@Summary		查询 Agent
//	@Description	按 agentId 返回该 Agent 的元信息
//	@Tags			Agent
//	@ID				getAgent
//	@Produce		json
//	@Param			agentId	path		string				true	"Agent ID"
//	@Success		200		{object}	response.Response	"AgentInfo"
//	@Failure		404		{object}	response.Response	"agent 不存在"
//	@Router			/api/agents/{agentId} [get]
func (h *Agent) Get(c *gin.Context) (response.Response, error) {
	id := c.Param("agentId")
	info, ok := h.s.Get(id)
	if !ok {
		return response.Response{}, errs.ErrAgentNotFound(nil)
	}
	return response.Response{
		Code:    0,
		Message: "success",
		Data:    info,
	}, nil
}
