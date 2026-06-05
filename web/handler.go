package web

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Serendipity565/gora/agent"
	"github.com/Serendipity565/gora/tool"
)

// ChatRequest 描述一次对话请求体。
type ChatRequest struct {
	Message   string `json:"message" binding:"required"`
	AgentID   string `json:"agent_id"`
	SessionID string `json:"session_id"`
}

// AgentInfo 是返回给前端的 Agent 元信息。
type AgentInfo struct {
	ID    string      `json:"id"`
	State agent.State `json:"-"`
	// 状态以字符串形式输出，方便前端渲染。
	StateText string `json:"state"`
	Type      string `json:"type"`
	Model     string `json:"model,omitempty"`
}

// ToolInfo 描述前端可见的工具元信息。
type ToolInfo struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

// AgentRunner 抽象 Agent 的运行能力，避免与具体类型耦合。
type AgentRunner interface {
	ID() string
	State() agent.State
	Run(ctx context.Context, input string) <-chan agent.Event
}

// SessionRunner 是支持多会话的 Agent，HandleChat 会优先调用 RunSession。
type SessionRunner interface {
	AgentRunner
	RunSession(ctx context.Context, sessionID, input string) <-chan agent.Event
}

// Handler 把 Gora Agent 暴露为 HTTP / SSE 接口。
type Handler struct {
	mu       sync.RWMutex
	agents   map[string]AgentRunner
	registry *tool.Registry

	model       string
	chatTimeout time.Duration
}

// HandlerOption 配置 Handler 的可选项。
type HandlerOption func(*Handler)

// WithModelName 设置当前默认模型名，用于 /api/agents 元数据。
func WithModelName(model string) HandlerOption {
	return func(h *Handler) {
		h.model = model
	}
}

// WithChatTimeout 设置单次对话的最大时长。
func WithChatTimeout(d time.Duration) HandlerOption {
	return func(h *Handler) {
		if d > 0 {
			h.chatTimeout = d
		}
	}
}

// NewHandler 创建一个 Handler。
func NewHandler(registry *tool.Registry, opts ...HandlerOption) *Handler {
	h := &Handler{
		agents:      make(map[string]AgentRunner),
		registry:    registry,
		chatTimeout: 5 * time.Minute,
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// RegisterAgent 把一个 Agent 加入 Handler，可热替换同 ID Agent。
func (h *Handler) RegisterAgent(a AgentRunner) {
	if a == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.agents[a.ID()] = a
}

// SetModel 在运行时更新当前模型名。
func (h *Handler) SetModel(model string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.model = model
}

// HandleChat 处理一次 SSE 流式对话。
func (h *Handler) HandleChat(c *gin.Context) {
	var req ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.AgentID = resolveRequestedAgentID(c, req.AgentID)
	req.SessionID = strings.TrimSpace(req.SessionID)

	a, ok := h.resolveAgent(req.AgentID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}

	sw, err := NewSSEWriter(c.Writer)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), h.chatTimeout)
	defer cancel()

	var events <-chan agent.Event
	if sessionRunner, ok := a.(SessionRunner); ok {
		events = sessionRunner.RunSession(ctx, req.SessionID, req.Message)
	} else {
		events = a.Run(ctx, req.Message)
	}

	terminalSent := false
	for event := range events {
		if err := sw.WriteEvent(event); err != nil {
			// 写入失败通常意味着客户端断开。
			cancel()
			break
		}
		if event.Type == agent.EventDone || event.Type == agent.EventError {
			terminalSent = true
			break
		}
	}

	if !terminalSent {
		_ = sw.WriteDone()
	}
}

// HandleListAgents 返回所有可用 Agent 的元信息。
func (h *Handler) HandleListAgents(c *gin.Context) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	infos := make([]AgentInfo, 0, len(h.agents))
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

// HandleListTools 列出所有已注册工具，用于前端展示。
func (h *Handler) HandleListTools(c *gin.Context) {
	if h.registry == nil {
		c.JSON(http.StatusOK, gin.H{"tools": []ToolInfo{}})
		return
	}

	tools := h.registry.List()
	infos := make([]ToolInfo, 0, len(tools))
	for _, t := range tools {
		infos = append(infos, ToolInfo{
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

// HandleIndex 返回内嵌前端页面。
func (h *Handler) HandleIndex(c *gin.Context) {
	page, err := IndexHTML()
	if err != nil {
		c.String(http.StatusInternalServerError, "load index page: %v", err)
		return
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", page)
}

func (h *Handler) resolveAgent(id string) (AgentRunner, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if id != "" {
		a, ok := h.agents[id]
		return a, ok
	}
	for _, a := range h.agents {
		return a, true
	}
	return nil, false
}

func (h *Handler) toAgentInfo(a AgentRunner) AgentInfo {
	state := a.State()
	return AgentInfo{
		ID:        a.ID(),
		State:     state,
		StateText: state.String(),
		Type:      detectAgentType(a),
		Model:     h.model,
	}
}

func detectAgentType(a AgentRunner) string {
	if _, ok := a.(*agent.EinoAgent); ok {
		return "eino"
	}
	if _, ok := a.(*agent.BaseAgent); ok {
		return "base"
	}
	return "unknown"
}

func resolveRequestedAgentID(c *gin.Context, bodyAgentID string) string {
	if agentID := strings.TrimSpace(c.Param("agentId")); agentID != "" {
		return agentID
	}
	if agentID := strings.TrimSpace(bodyAgentID); agentID != "" {
		return agentID
	}
	return ""
}
