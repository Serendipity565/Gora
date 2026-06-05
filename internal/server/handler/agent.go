package handler

import (
	"net/http"
	"sort"

	"github.com/gin-gonic/gin"

	"github.com/Serendipity565/gora/internal/agent/core"
	"github.com/Serendipity565/gora/internal/agent/eino"
	"github.com/Serendipity565/gora/internal/server/api/v1/response"
)

// HandleListAgents 返回所有可用 Agent 的元信息。
func (h *Handler) HandleListAgents(c *gin.Context) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	infos := make([]response.AgentInfo, 0, len(h.agents))
	for _, a := range h.agents {
		infos = append(infos, h.toAgentInfo(a))
	}
	sort.Slice(infos, func(i, j int) bool {
		return infos[i].ID < infos[j].ID
	})
	c.JSON(http.StatusOK, gin.H{"agents": infos})
}

// HandleGetAgent 返回单个 Agent 状态。
func (h *Handler) HandleGetAgent(c *gin.Context) {
	id := c.Param("agentId")
	h.mu.RLock()
	a, ok := h.agents[id]
	h.mu.RUnlock()
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}
	c.JSON(http.StatusOK, h.toAgentInfo(a))
}

func (h *Handler) toAgentInfo(a AgentRunner) response.AgentInfo {
	state := a.State()
	return response.AgentInfo{
		ID:        a.ID(),
		State:     state,
		StateText: state.String(),
		Type:      detectAgentType(a),
		Model:     h.model,
	}
}

// detectAgentType 通过运行期类型断言识别 Agent 实现，仅用于前端展示。
func detectAgentType(a AgentRunner) string {
	if _, ok := a.(*eino.EinoAgent); ok {
		return "eino"
	}
	if _, ok := a.(*core.BaseAgent); ok {
		return "base"
	}
	return "unknown"
}
