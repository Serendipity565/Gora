package app

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/cloudwego/eino/schema"

	"github.com/Serendipity565/gora/internal/agent/core"
	"github.com/Serendipity565/gora/internal/agent/tool"
	appconfig "github.com/Serendipity565/gora/internal/config"
	storage "github.com/Serendipity565/gora/internal/repository"
	"github.com/Serendipity565/gora/internal/server"
)

// Runner 是 Gora 主入口注册到 server.Handler 的多会话 Agent 运行器。
//
// 它实现了：
//   - server.SessionRunner：controller.HandleChat 据此驱动 SSE 对话
//   - server.ModelSelector：controller.HandleListModels / HandleSelectModel 据此读写模型选择
//
// 状态语义（State()）：
//   - StateIdle    刚创建 / 尚未运行
//   - StateRunning 正在思考 / 正在流式输出
//   - StateWaiting 工具调用进行中
//   - StateDone    本轮成功结束
//   - StateError   本轮异常结束
type Runner struct {
	mu sync.RWMutex

	cfg          appconfig.Config
	registry     *tool.Registry
	historyStore storage.ChatHistoryStore
	modelStore   storage.ModelSelectionStore
	memoryStore  storage.ActiveMemoryStore
	userID       string
	llmIndex     int
	forceModel   bool
	state        core.State
	histories    map[string][]*schema.Message
}

// NewRunner 创建一个 Runner。
//
//   - llmIndex：当前会话默认的 LLM 索引；forceModel=true 时锁死，否则会被 modelStore 中的选择覆盖。
//   - forceModel：通常对应"启动时显式指定模型"的场景；本仓库当前不通过 flag 暴露这个开关，
//     保留参数是为了未来支持 GORA_FORCE_MODEL 之类的约束。
func NewRunner(
	cfg appconfig.Config,
	registry *tool.Registry,
	modelStore storage.ModelSelectionStore,
	historyStore storage.ChatHistoryStore,
	memoryStore storage.ActiveMemoryStore,
	userID string,
	llmIndex int,
	forceModel bool,
) *Runner {
	return &Runner{
		cfg:          cfg,
		registry:     registry,
		modelStore:   modelStore,
		historyStore: historyStore,
		memoryStore:  memoryStore,
		userID:       userID,
		llmIndex:     llmIndex,
		forceModel:   forceModel,
		state:        core.StateIdle,
		histories:    make(map[string][]*schema.Message),
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

// Run 使用默认会话执行一轮对话。
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

		sessionID = normalizeSessionID(sessionID)
		llmIndex := r.resolveLLMIndex(ctx, sessionID)

		chatAgent, err := buildChatAgent(ctx, r.cfg, r.registry, llmIndex, nil)
		if err != nil {
			r.setState(core.StateError)
			output <- core.NewErrorEvent(r.ID(), fmt.Errorf("创建 Agent 失败: %w", err))
			return
		}

		chatAgent.RestoreHistories(r.snapshotHistories())
		restoreConversationContext(ctx, io.Discard, io.Discard, r.historyStore, r.memoryStore, r.userID, sessionID, r.cfg, chatAgent)
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
			persistConversationTurn(ctx, io.Discard, r.historyStore, r.memoryStore, r.userID, sessionID, r.cfg, llmIndex, input, assistantReply.String(), chatAgent)
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

// PrimeHistories 在启动时把 chatAgent 的初始历史覆盖到 Runner，避免首次 RunSession
// 跑出空上下文。典型用法：Run 装配阶段创建一次 chatAgent，把它的 (空) 快照灌进去做基线。
func (r *Runner) PrimeHistories(histories map[string][]*schema.Message) {
	r.replaceHistories(histories)
}

func (r *Runner) resolveLLMIndex(ctx context.Context, sessionID string) int {
	if r.forceModel {
		return r.llmIndex
	}
	return restoreModelSelection(ctx, io.Discard, io.Discard, r.modelStore, r.userID, sessionID, r.cfg)
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
			Provider: llmConfig.Provider,
			Model:    llmConfig.Model,
			Display:  llmConfig.Name,
		})
	}
	return models
}

// CurrentModel 返回某 session 当前使用的模型。
//
// forceModel=true 时永远返回锁定的 r.llmIndex，且 explicit=true。
func (r *Runner) CurrentModel(ctx context.Context, sessionID string) (server.ModelInfo, bool, error) {
	if r.forceModel {
		return r.modelInfoAt(r.llmIndex), true, nil
	}

	sessionID = normalizeSessionID(sessionID)
	selection, ok, err := r.modelStore.Get(ctx, r.userID, r.cfg.Agent.ID, sessionID)
	if err != nil {
		return server.ModelInfo{}, false, err
	}
	if !ok {
		return r.modelInfoAt(0), false, nil
	}

	index, ok := resolveStoredLLMIndex(r.cfg, selection)
	if !ok {
		return r.modelInfoAt(0), false, nil
	}
	return r.modelInfoAt(index), true, nil
}

// SelectModel 把 name 解析为模型并持久化，
// 后续该 session 的 RunSession 会自动读取这一选择。
//
// 入参 name 即 LLMConfig.Name；项目约定 name 是模型的唯一标识。
func (r *Runner) SelectModel(ctx context.Context, sessionID, name string) (server.ModelInfo, error) {
	if r.forceModel {
		return server.ModelInfo{}, fmt.Errorf("当前以锁定模式启动，禁止运行时切换模型")
	}
	if strings.TrimSpace(name) == "" {
		return server.ModelInfo{}, fmt.Errorf("模型 name 不能为空")
	}
	index, _, err := r.cfg.FindLLM(name)
	if err != nil {
		return server.ModelInfo{}, err
	}

	sessionID = normalizeSessionID(sessionID)
	saveModelSelection(ctx, io.Discard, r.modelStore, r.userID, sessionID, r.cfg, index)
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
		Provider: llmConfig.Provider,
		Model:    llmConfig.Model,
		Display:  llmConfig.Name,
	}
}
