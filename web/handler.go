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
	// DisabledTools 是用户在前端 UI 中关闭的工具名集合，
	// HandleChat 会通过 context 把它透传给 Agent，命中名单的工具会被直接拒绝执行。
	DisabledTools []string `json:"disabled_tools,omitempty"`
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

// ModelInfo 是前端可见的可选模型项。
type ModelInfo struct {
	Index    int    `json:"index"`
	Name     string `json:"name,omitempty"`
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model"`
	Display  string `json:"display"`
}

// ModelSelector 抽象"列出 / 读取 / 切换模型"的能力，
// 由 cmd 层基于 sessionAgentRunner + ModelSelectionStore 实现。
type ModelSelector interface {
	// ListModels 返回当前配置中所有可用模型，顺序与配置一致。
	ListModels() []ModelInfo
	// CurrentModel 返回给定 session 当前使用的模型；
	// session 为空表示默认会话。第二个返回值表示 session 是否有显式选择，
	// 没有时返回的 ModelInfo 是回退使用的默认值。
	CurrentModel(ctx context.Context, sessionID string) (ModelInfo, bool, error)
	// SelectModel 把 selector（序号 / name / model）转换为索引并持久化为 session 的选择，
	// 返回最终生效的 ModelInfo。
	SelectModel(ctx context.Context, sessionID, selector string) (ModelInfo, error)
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

	// permissionGate 在所有 chat 会话间共享，前端通过 HandleToolPermission
	// 把"是否允许调用某个被禁用的工具"的决定写回给等待中的 Agent。
	permissionGate *agent.ToolPermissionGate

	// modelSelector 提供"列模型 / 切模型"的能力；nil 时模型相关接口返回 501。
	modelSelector ModelSelector
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

// WithModelSelector 注入一个 ModelSelector，启用 /api/models 系列接口。
// 不调用时模型接口会返回 501，前端可据此隐藏模型选择 UI。
func WithModelSelector(selector ModelSelector) HandlerOption {
	return func(h *Handler) {
		h.modelSelector = selector
	}
}

// NewHandler 创建一个 Handler。
func NewHandler(registry *tool.Registry, opts ...HandlerOption) *Handler {
	h := &Handler{
		agents:         make(map[string]AgentRunner),
		registry:       registry,
		chatTimeout:    5 * time.Minute,
		permissionGate: agent.NewToolPermissionGate(),
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

	// 把"被用户关闭的工具"集合附加到 context，
	// 让 Agent 在 InvokableRun 中按需拒绝执行 / 发起授权询问。
	ctx = agent.WithDisabledTools(ctx, req.DisabledTools)
	ctx = agent.WithToolPermissionGate(ctx, h.permissionGate)

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

// ModelSelectRequest 描述一次模型切换请求体。
type ModelSelectRequest struct {
	// SessionID 为空时使用默认会话。
	SessionID string `json:"session_id"`
	// Selector 可以是序号（"1"）、name 或 model 字符串。
	Selector string `json:"selector" binding:"required"`
}

// HandleListModels 列出当前配置中所有可选模型。
func (h *Handler) HandleListModels(c *gin.Context) {
	if h.modelSelector == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "model selector not configured"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"models": h.modelSelector.ListModels()})
}

// HandleGetCurrentModel 返回某会话当前使用的模型。
// session_id 通过 query 传入；为空表示默认会话。
func (h *Handler) HandleGetCurrentModel(c *gin.Context) {
	if h.modelSelector == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "model selector not configured"})
		return
	}

	sessionID := strings.TrimSpace(c.Query("session_id"))
	info, explicit, err := h.modelSelector.CurrentModel(c.Request.Context(), sessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"model":    info,
		"explicit": explicit,
	})
}

// HandleSelectModel 设置某会话使用的模型。
func (h *Handler) HandleSelectModel(c *gin.Context) {
	if h.modelSelector == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "model selector not configured"})
		return
	}

	var req ModelSelectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	info, err := h.modelSelector.SelectModel(c.Request.Context(), strings.TrimSpace(req.SessionID), strings.TrimSpace(req.Selector))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"model": info})
}

// ToolPermissionRequestBody 描述前端确认/拒绝某次工具授权的请求体。
type ToolPermissionRequestBody struct {
	RequestID string `json:"request_id" binding:"required"`
	Approve   bool   `json:"approve"`
	Remember  bool   `json:"remember,omitempty"`
}

// HandleToolPermission 由前端在弹窗中得到用户决定后调用，
// 把决定写回正在等待的 Agent goroutine。
func (h *Handler) HandleToolPermission(c *gin.Context) {
	var req ToolPermissionRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if h.permissionGate == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "permission gate not configured"})
		return
	}

	resolved := h.permissionGate.Resolve(req.RequestID, agent.ToolPermissionDecision{
		Approved: req.Approve,
		Remember: req.Remember,
	})
	if !resolved {
		// request_id 已过期 / 不存在（典型情况：用户点击太晚或 Agent 已被取消）。
		c.JSON(http.StatusNotFound, gin.H{"error": "permission request not found or already resolved"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"resolved": true})
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
