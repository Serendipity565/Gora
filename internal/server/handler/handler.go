// Package handler 是 Gora HTTP/SSE API 的请求处理层。
//
// 这一层是 Gin 与业务的边界：解析请求体、调用底层 Agent / Tool / ModelSelector，
// 把结果序列化为 JSON / SSE 帧返回给前端。具体业务逻辑（编排、持久化）应进一步
// 下沉到 service 层 / agent 子包，handler 只做轻量装配。
package handler

import (
	"context"
	"sync"
	"time"

	"github.com/Serendipity565/gora/internal/agent/core"
	"github.com/Serendipity565/gora/internal/agent/tool"
	"github.com/Serendipity565/gora/internal/server/api/v1/response"
	"github.com/Serendipity565/gora/internal/server/service"
)

// AgentRunner 抽象 Agent 的运行能力，避免与具体类型耦合。
type AgentRunner interface {
	ID() string
	State() core.State
	Run(ctx context.Context, input string) <-chan core.Event
}

// SessionRunner 是支持多会话的 Agent，HandleChat 会优先调用 RunSession。
type SessionRunner interface {
	AgentRunner
	RunSession(ctx context.Context, sessionID, input string) <-chan core.Event
}

// ModelSelector 抽象 "列出 / 读取 / 切换模型" 的能力，
// 由上层（cli 装配代码）基于 sessionAgentRunner + ModelSelectionStore 实现。
type ModelSelector interface {
	ListModels() []response.ModelInfo
	CurrentModel(ctx context.Context, sessionID string) (response.ModelInfo, bool, error)
	SelectModel(ctx context.Context, sessionID, selector string) (response.ModelInfo, error)
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
	permissionGate *core.ToolPermissionGate

	// modelSelector 提供 "列模型 / 切模型" 的能力；nil 时模型相关接口返回 501。
	modelSelector ModelSelector

	// chatService 把 SSE 流式编排从 handler 抽离出来，
	// handler 只负责 HTTP 解码 / 选超时 / 包 sink，业务流转交给 service。
	chatService *service.ChatService
}

// Option 配置 Handler 的可选项。
type Option func(*Handler)

// WithModelName 设置当前默认模型名，用于 /api/agents 元数据。
func WithModelName(model string) Option {
	return func(h *Handler) {
		h.model = model
	}
}

// WithChatTimeout 设置单次对话的最大时长。
func WithChatTimeout(d time.Duration) Option {
	return func(h *Handler) {
		if d > 0 {
			h.chatTimeout = d
		}
	}
}

// WithModelSelector 注入一个 ModelSelector，启用 /api/models 系列接口。
// 不调用时模型接口会返回 501，前端可据此隐藏模型选择 UI。
func WithModelSelector(selector ModelSelector) Option {
	return func(h *Handler) {
		h.modelSelector = selector
	}
}

// New 创建一个 Handler。
func New(registry *tool.Registry, opts ...Option) *Handler {
	h := &Handler{
		agents:         make(map[string]AgentRunner),
		registry:       registry,
		chatTimeout:    5 * time.Minute,
		permissionGate: core.NewToolPermissionGate(),
	}
	for _, opt := range opts {
		opt(h)
	}
	h.chatService = service.NewChatService(handlerAgentResolver{h: h}, h.permissionGate)
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

// resolveAgent 在 agents 表中查找指定 ID；id 为空时返回任一 Agent。
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

// handlerAgentResolver 把 Handler 包装成 service.AgentResolver。
//
// service 包对 handler 类型一无所知，只需要 "按 ID 找 Agent" 这一能力。
type handlerAgentResolver struct {
	h *Handler
}

func (r handlerAgentResolver) Resolve(id string) (service.AgentRunner, bool) {
	a, ok := r.h.resolveAgent(id)
	if !ok {
		return nil, false
	}
	return a, true
}
