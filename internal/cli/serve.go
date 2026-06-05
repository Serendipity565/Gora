package cli

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/spf13/cobra"

	"github.com/Serendipity565/gora/internal/agent/core"
	"github.com/Serendipity565/gora/internal/agent/eino"
	"github.com/Serendipity565/gora/internal/agent/tool"
	appconfig "github.com/Serendipity565/gora/internal/config"
	storage "github.com/Serendipity565/gora/internal/repository"
	"github.com/Serendipity565/gora/internal/server"
	"github.com/Serendipity565/gora/internal/wired"
)

// serveOptions 控制 web 服务的运行参数。
//
// Gora 后端只提供 JSON / SSE API；前端是独立的 Vite 项目，通过浏览器直接访问 :8080。
// 因此默认开启 CORS，允许 vite dev server / 任意 origin 的前端跨域访问。
type serveOptions struct {
	CORS bool
}

var serverOptions = serveOptions{
	CORS: true,
}

type sessionAgentRunner struct {
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

func newSessionAgentRunner(
	cfg appconfig.Config,
	registry *tool.Registry,
	modelStore storage.ModelSelectionStore,
	historyStore storage.ChatHistoryStore,
	memoryStore storage.ActiveMemoryStore,
	userID string,
	llmIndex int,
	forceModel bool,
) *sessionAgentRunner {
	return &sessionAgentRunner{
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

func (r *sessionAgentRunner) ID() string {
	return r.cfg.Agent.ID
}

func (r *sessionAgentRunner) State() core.State {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.state
}

func (r *sessionAgentRunner) Run(ctx context.Context, input string) <-chan core.Event {
	return r.RunSession(ctx, eino.DefaultSessionID, input)
}

func (r *sessionAgentRunner) RunSession(ctx context.Context, sessionID, input string) <-chan core.Event {
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
		var gotDone bool
		var gotError bool

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

func (r *sessionAgentRunner) resolveLLMIndex(ctx context.Context, sessionID string) int {
	if r.forceModel {
		return r.llmIndex
	}
	return restoreModelSelection(ctx, io.Discard, io.Discard, r.modelStore, r.userID, sessionID, r.cfg)
}

func (r *sessionAgentRunner) snapshotHistories() map[string][]*schema.Message {
	r.mu.RLock()
	defer r.mu.RUnlock()

	histories := make(map[string][]*schema.Message, len(r.histories))
	for sessionID, history := range r.histories {
		histories[sessionID] = append([]*schema.Message(nil), history...)
	}
	return histories
}

func (r *sessionAgentRunner) replaceHistories(histories map[string][]*schema.Message) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.histories = make(map[string][]*schema.Message, len(histories))
	for sessionID, history := range histories {
		r.histories[sessionID] = append([]*schema.Message(nil), history...)
	}
}

func (r *sessionAgentRunner) setState(state core.State) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.state = state
}

// ListModels 暴露当前配置中所有可选模型，供 web 前端展示。
func (r *sessionAgentRunner) ListModels() []server.ModelInfo {
	models := make([]server.ModelInfo, 0, len(r.cfg.LLM))
	for index, llmConfig := range r.cfg.LLM {
		models = append(models, server.ModelInfo{
			Index:    index,
			Name:     llmConfig.Name,
			Provider: llmConfig.Provider,
			Model:    llmConfig.Model,
			Display:  llmConfig.DisplayName(index),
		})
	}
	return models
}

// CurrentModel 返回某 session 当前使用的模型。
// 如果该 session 没有显式选择，回退到默认（cfg.LLM[0]），并把 explicit=false 告知调用方。
//   - r.forceModel 为 true 时表示用户在启动时通过 --model 锁定了模型，
//     此时永远返回 r.llmIndex 对应的模型，且 explicit=true（视为锁定）。
func (r *sessionAgentRunner) CurrentModel(ctx context.Context, sessionID string) (server.ModelInfo, bool, error) {
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

// SelectModel 把 selector（序号 / name / model）解析为模型并持久化，
// 后续该 session 的 RunSession 会自动读取这一选择。
func (r *sessionAgentRunner) SelectModel(ctx context.Context, sessionID, selector string) (server.ModelInfo, error) {
	if r.forceModel {
		return server.ModelInfo{}, fmt.Errorf("当前以 --model 锁定模式启动，禁止运行时切换模型")
	}
	if strings.TrimSpace(selector) == "" {
		return server.ModelInfo{}, fmt.Errorf("selector 不能为空")
	}
	index, _, err := r.cfg.FindLLM(selector)
	if err != nil {
		return server.ModelInfo{}, err
	}

	sessionID = normalizeSessionID(sessionID)
	saveModelSelection(ctx, io.Discard, r.modelStore, r.userID, sessionID, r.cfg, index)
	return r.modelInfoAt(index), nil
}

// modelInfoAt 把 cfg.LLM[index] 转换成对外暴露的 ModelInfo；
// index 越界时回落到 0，避免 panic。
func (r *sessionAgentRunner) modelInfoAt(index int) server.ModelInfo {
	if index < 0 || index >= len(r.cfg.LLM) {
		index = 0
	}
	llmConfig := r.cfg.LLM[index]
	return server.ModelInfo{
		Index:    index,
		Name:     llmConfig.Name,
		Provider: llmConfig.Provider,
		Model:    llmConfig.Model,
		Display:  llmConfig.DisplayName(index),
	}
}

func runServerCommand(command *cobra.Command, _ []string) error {
	return runServer(command.Context(), command.OutOrStdout(), command.ErrOrStderr(), options, serverOptions)
}

func runServer(parent context.Context, out, errOut io.Writer, opts cliOptions, serveOpts serveOptions) error {
	if parent == nil {
		parent = context.Background()
	}

	cfg, err := appconfig.Read(opts.ConfigPath)
	if err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}
	applyRuntimeOverrides(&cfg, opts)
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("配置校验失败: %w", err)
	}

	forceModel := strings.TrimSpace(opts.Model) != ""
	infra, cleanup, err := wired.InitInfra(parent, cfg)
	if err != nil {
		return fmt.Errorf("初始化基础设施失败: %w", err)
	}
	defer cleanup()

	registry := infra.Registry
	modelStore := infra.DB
	memoryStore := infra.Cache
	userID := modelSelectionUserID(opts)

	currentLLMIndex := 0
	if !forceModel {
		currentLLMIndex = restoreModelSelection(parent, io.Discard, io.Discard, modelStore, userID, eino.DefaultSessionID, cfg)
	}

	runner := newSessionAgentRunner(
		cfg,
		registry,
		modelStore,
		modelStore,
		memoryStore,
		userID,
		currentLLMIndex,
		forceModel,
	)

	chatAgent, err := buildChatAgent(parent, cfg, registry, currentLLMIndex, nil)
	if err != nil {
		return fmt.Errorf("创建 Agent 失败: %w", err)
	}
	runner.replaceHistories(chatAgent.SnapshotHistories())

	handler := server.NewHandler(
		registry,
		server.WithModelName(cfg.LLM[0].DisplayName(0)),
		server.WithModelSelector(runner),
	)
	handler.RegisterAgent(runner)

	router := server.NewRouter(handler, server.RouterOptions{CORS: serveOpts.CORS})

	addr := normalizeAddr(opts.ServerAddr)
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           router,
		ReadHeaderTimeout: 15 * time.Second,
	}

	fmt.Fprintf(out, "🚀 服务监听: http://%s\n", displayAddr(addr))
	fmt.Fprintf(out, "🤖 Agent: %s (模型: %s)\n", runner.ID(), cfg.LLM[currentLLMIndex].DisplayName(currentLLMIndex))
	fmt.Fprintf(out, "🔧 已加载工具: %d 个\n", len(registry.List()))
	fmt.Fprintln(out, "🗂  纯 API 服务（无内嵌前端）：使用 frontend/ 单独启动 UI，或访问 /api/* 与 /health")
	fmt.Fprintln(out, "按 Ctrl+C 退出")

	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	errCh := make(chan error, 1)
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-sigCh:
		fmt.Fprintln(out, "\n👋 正在关闭...")
	case <-ctx.Done():
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("HTTP 服务异常退出: %w", err)
		}
		return nil
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		fmt.Fprintf(errOut, "关闭服务失败: %v\n", err)
	}
	return nil
}

func normalizeAddr(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return ":8080"
	}
	if !strings.Contains(addr, ":") {
		return ":" + addr
	}
	return addr
}

func displayAddr(addr string) string {
	if strings.HasPrefix(addr, ":") {
		return "localhost" + addr
	}
	return addr
}

func init() {
	rootCmd.Flags().BoolVar(&serverOptions.CORS, "cors", serverOptions.CORS, "是否开启简单 CORS（前端跨域访问后端时必需）")
}
