// Package runner 实现 Gora 的多会话 Agent 运行器。
//
// Runner 同时实现 server.SessionRunner 与 server.ModelSelector，用于挂到
// AgentService / ModelService 上。它依赖运行时配置（cfg）才能起，
// 因此不在 wire 阶段构造，而是在 main 里手动 New 出来再注入到 service。
package runner

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	appconfig "github.com/Serendipity565/gora/configs"
	"github.com/cloudwego/eino/schema"

	"github.com/Serendipity565/gora/internal/agent/core"
	"github.com/Serendipity565/gora/internal/agent/eino"
	"github.com/Serendipity565/gora/internal/agent/tool"
	"github.com/Serendipity565/gora/internal/repository"
	"github.com/Serendipity565/gora/internal/repository/model"
	"github.com/Serendipity565/gora/internal/server"
)

// Runner 是注册到 AgentService 的多会话 Agent 运行器。
//
// 状态语义（State()）：
//   - StateIdle    刚创建 / 尚未运行
//   - StateRunning 正在思考 / 正在流式输出
//   - StateWaiting 工具调用进行中
//   - StateDone    本轮成功结束
//   - StateError   本轮异常结束
type Runner struct {
	mu sync.RWMutex

	cfg         appconfig.Config
	registry    *tool.Registry
	sessionDAO  repository.SessionDAO
	messageDAO  repository.MessageDAO
	memoryStore repository.ActiveMemoryStore
	userID      uint64
	llmIndex    int
	state       core.State
	histories   map[string][]*schema.Message

	// sessionLLM 缓存 (session → llm_name) 选择，供 ListModels / SelectModel 使用。
	// 真正持久化是写在 Session.LLMName 上；这里仅做内存层 fast-path。
	sessionLLM map[string]string
}

// New 创建一个 Runner。
//
//	llmIndex 是当前会话默认的 LLM 索引；运行时会被 session.llm_name 覆盖（如果有）。
func New(
	cfg appconfig.Config,
	registry *tool.Registry,
	sessionDAO repository.SessionDAO,
	messageDAO repository.MessageDAO,
	memoryStore repository.ActiveMemoryStore,
	llmIndex int,
) *Runner {
	return &Runner{
		cfg:         cfg,
		registry:    registry,
		sessionDAO:  sessionDAO,
		messageDAO:  messageDAO,
		memoryStore: memoryStore,
		userID:      0,
		llmIndex:    llmIndex,
		state:       core.StateIdle,
		histories:   make(map[string][]*schema.Message),
		sessionLLM:  make(map[string]string),
	}
}

func (r *Runner) ID() string {
	return r.cfg.Agent.ID
}

func (r *Runner) State() core.State {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.state
}

// Run 在调用方未指定 session 的情况下报错——Gora 不再有 "default" 兜底。
func (r *Runner) Run(ctx context.Context, input string) <-chan core.Event {
	return r.RunSession(ctx, "", input)
}

// RunSession 在指定 session 中执行一轮对话。
//
// 内部流程：解析当前 LLM → 构造一次性 chatAgent → 恢复历史 → 消费事件 → 落库 + 回写短期记忆。
func (r *Runner) RunSession(ctx context.Context, sessionID, input string) <-chan core.Event {
	output := make(chan core.Event, 32)

	go func() {
		defer close(output)

		sessionID, err := requireSessionID(sessionID)
		if err != nil {
			r.setState(core.StateError)
			output <- core.NewErrorEvent(r.ID(), err)
			return
		}
		llmIndex := r.resolveLLMIndex(ctx, sessionID)

		chatAgent, err := buildChatAgent(ctx, r.cfg, r.registry, llmIndex, nil)
		if err != nil {
			r.setState(core.StateError)
			output <- core.NewErrorEvent(r.ID(), fmt.Errorf("创建 Agent 失败: %w", err))
			return
		}

		chatAgent.RestoreHistories(r.snapshotHistories())
		restoreConversationContext(ctx, io.Discard, io.Discard, r.messageDAO, r.memoryStore, r.userID, sessionID, r.cfg, chatAgent)
		r.setState(core.StateRunning)

		var assistantReply strings.Builder
		var gotDone, gotError bool

		for event := range chatAgent.RunSession(ctx, sessionID, input) {
			switch event.Type {
			case core.EventToolCall:
				r.setState(core.StateWaiting)
			case core.EventThinking, core.EventToolResult, core.EventChunk:
				r.setState(core.StateRunning)
			case core.EventDone:
				gotDone = true
				r.setState(core.StateDone)
			case core.EventError:
				gotError = true
				r.setState(core.StateError)
			}

			if event.Type == core.EventChunk {
				assistantReply.WriteString(event.Content)
			}
			output <- event
		}

		r.replaceHistories(chatAgent.SnapshotHistories())
		if gotDone && !gotError {
			persistConversationTurn(ctx, io.Discard, r.sessionDAO, r.messageDAO, r.memoryStore, r.userID, sessionID, r.cfg, llmIndex, input, assistantReply.String(), chatAgent, time.Now)
		}
		if !gotDone && !gotError {
			r.setState(core.StateIdle)
		}
	}()

	return output
}

// Stop 暂未实现：当前 Runner 是无状态短轮次模型，外部 ctx 取消即可终止 RunSession 内部循环。
func (r *Runner) Stop() error {
	return nil
}

// PrimeHistories 在启动时把 chatAgent 的初始历史覆盖到 Runner，
// 避免首次 RunSession 跑出空上下文。
func (r *Runner) PrimeHistories(histories map[string][]*schema.Message) {
	r.replaceHistories(histories)
}

// BuildBootstrapAgent 用启动时的 LLMIndex 临时构造一个 EinoAgent，
// 调用方据此 PrimeHistories；构造失败直接返回错误。
func (r *Runner) BuildBootstrapAgent(ctx context.Context) (*eino.EinoAgent, error) {
	return buildChatAgent(ctx, r.cfg, r.registry, r.llmIndex, nil)
}

// LLMIndex 返回 Runner 启动时落到的 LLM 索引（典型用于打日志展示当前模型名）。
func (r *Runner) LLMIndex() int {
	return r.llmIndex
}

// resolveLLMIndex 读取 session 持久化的 llm_name；
// 找不到则回落到 Runner 默认 llmIndex。
func (r *Runner) resolveLLMIndex(ctx context.Context, sessionID string) int {
	r.mu.RLock()
	if cached, ok := r.sessionLLM[sessionID]; ok {
		r.mu.RUnlock()
		if index, ok := resolveStoredLLMIndex(r.cfg, cached); ok {
			return index
		}
		return r.llmIndex
	}
	r.mu.RUnlock()

	session, err := r.sessionDAO.FindOne(ctx, repository.BySessionID(sessionID))
	if err != nil || session == nil {
		return r.llmIndex
	}
	r.mu.Lock()
	r.sessionLLM[sessionID] = session.LLMName
	r.mu.Unlock()

	if index, ok := resolveStoredLLMIndex(r.cfg, session.LLMName); ok {
		return index
	}
	return r.llmIndex
}

func (r *Runner) snapshotHistories() map[string][]*schema.Message {
	r.mu.RLock()
	defer r.mu.RUnlock()

	histories := make(map[string][]*schema.Message, len(r.histories))
	for sessionID, history := range r.histories {
		histories[sessionID] = append([]*schema.Message(nil), history...)
	}
	return histories
}

func (r *Runner) replaceHistories(histories map[string][]*schema.Message) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.histories = make(map[string][]*schema.Message, len(histories))
	for sessionID, history := range histories {
		r.histories[sessionID] = append([]*schema.Message(nil), history...)
	}
}

func (r *Runner) setState(state core.State) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.state = state
}

// ListModels 暴露当前配置中所有可选模型，供前端展示。
func (r *Runner) ListModels() []server.ModelInfo {
	models := make([]server.ModelInfo, 0, len(r.cfg.LLM))
	for index, llmConfig := range r.cfg.LLM {
		models = append(models, server.ModelInfo{
			Index:    index,
			Name:     llmConfig.Name,
			APIStyle: llmConfig.APIStyle,
			Model:    llmConfig.Model,
			Display:  llmConfig.Name,
		})
	}
	return models
}

// CurrentModel 返回某 session 当前使用的模型。
func (r *Runner) CurrentModel(ctx context.Context, sessionID string) (server.ModelInfo, bool, error) {
	sessionID, err := requireSessionID(sessionID)
	if err != nil {
		return server.ModelInfo{}, false, err
	}

	session, err := r.sessionDAO.FindOne(ctx, repository.BySessionID(sessionID))
	if err != nil {
		return server.ModelInfo{}, false, err
	}
	if session == nil {
		return r.modelInfoAt(0), false, nil
	}

	index, ok := resolveStoredLLMIndex(r.cfg, session.LLMName)
	if !ok {
		return r.modelInfoAt(0), false, nil
	}
	return r.modelInfoAt(index), true, nil
}

// SelectModel 把 name 解析为模型并持久化到 Session.LLMName，
// 后续该 session 的 RunSession 会自动读取这一选择。
//
// 入参 name 即 LLMConfig.Name；项目约定 name 是模型的唯一标识。
func (r *Runner) SelectModel(ctx context.Context, sessionID, name string) (server.ModelInfo, error) {
	if strings.TrimSpace(name) == "" {
		return server.ModelInfo{}, fmt.Errorf("模型 name 不能为空")
	}
	index, _, err := r.cfg.FindLLM(name)
	if err != nil {
		return server.ModelInfo{}, err
	}

	sessionID, err = requireSessionID(sessionID)
	if err != nil {
		return server.ModelInfo{}, err
	}

	llmConfig := r.cfg.LLM[index]
	if err := r.sessionDAO.Upsert(ctx, &model.Session{
		ID:            sessionID,
		UserID:        r.userID,
		Title:         "新对话",
		LLMName:       llmConfig.Name,
		LastMessageAt: time.Now(),
		Status:        model.SessionStatusActive,
	}); err != nil {
		return server.ModelInfo{}, fmt.Errorf("保存模型选择失败: %w", err)
	}

	r.mu.Lock()
	r.sessionLLM[sessionID] = llmConfig.Name
	r.mu.Unlock()

	return r.modelInfoAt(index), nil
}

func (r *Runner) modelInfoAt(index int) server.ModelInfo {
	if index < 0 || index >= len(r.cfg.LLM) {
		index = 0
	}
	llmConfig := r.cfg.LLM[index]
	return server.ModelInfo{
		Index:    index,
		Name:     llmConfig.Name,
		APIStyle: llmConfig.APIStyle,
		Model:    llmConfig.Model,
		Display:  llmConfig.Name,
	}
}
