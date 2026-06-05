package cmd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/gin-gonic/gin"
	"github.com/spf13/cobra"

	"github.com/Serendipity565/gora/agent"
	appconfig "github.com/Serendipity565/gora/config"
	"github.com/Serendipity565/gora/storage"
	"github.com/Serendipity565/gora/tool"
	"github.com/Serendipity565/gora/tool/builtin"
	"github.com/Serendipity565/gora/web"
)

// serveOptions 控制 web 服务的运行参数。
type serveOptions struct {
	StaticDir string
	CORS      bool
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
	state        agent.State
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
		state:        agent.StateIdle,
		histories:    make(map[string][]*schema.Message),
	}
}

func (r *sessionAgentRunner) ID() string {
	return r.cfg.Agent.ID
}

func (r *sessionAgentRunner) State() agent.State {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.state
}

func (r *sessionAgentRunner) Run(ctx context.Context, input string) <-chan agent.Event {
	return r.RunSession(ctx, agent.DefaultSessionID, input)
}

func (r *sessionAgentRunner) RunSession(ctx context.Context, sessionID, input string) <-chan agent.Event {
	output := make(chan agent.Event, 32)

	go func() {
		defer close(output)

		sessionID = normalizeSessionID(sessionID)
		llmIndex := r.resolveLLMIndex(ctx, sessionID)

		chatAgent, err := buildChatAgent(ctx, r.cfg, r.registry, llmIndex, nil)
		if err != nil {
			r.setState(agent.StateError)
			output <- agent.NewErrorEvent(r.ID(), fmt.Errorf("创建 Agent 失败: %w", err))
			return
		}

		chatAgent.RestoreHistories(r.snapshotHistories())
		restoreConversationContext(ctx, io.Discard, io.Discard, r.historyStore, r.memoryStore, r.userID, sessionID, r.cfg, chatAgent)
		r.setState(agent.StateRunning)

		var assistantReply strings.Builder
		var gotDone bool
		var gotError bool

		for event := range chatAgent.RunSession(ctx, sessionID, input) {
			switch event.Type {
			case agent.EventToolCall:
				r.setState(agent.StateWaiting)
			case agent.EventThinking, agent.EventToolResult, agent.EventChunk:
				r.setState(agent.StateRunning)
			case agent.EventDone:
				gotDone = true
				r.setState(agent.StateDone)
			case agent.EventError:
				gotError = true
				r.setState(agent.StateError)
			}

			if event.Type == agent.EventChunk {
				assistantReply.WriteString(event.Content)
			}
			output <- event
		}

		r.replaceHistories(chatAgent.SnapshotHistories())
		if gotDone && !gotError {
			persistConversationTurn(ctx, io.Discard, r.historyStore, r.memoryStore, r.userID, sessionID, r.cfg, llmIndex, input, assistantReply.String(), chatAgent)
		}
		if !gotDone && !gotError {
			r.setState(agent.StateIdle)
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

func (r *sessionAgentRunner) setState(state agent.State) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.state = state
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

	registry := tool.NewRegistry()
	if err := registry.Register(builtin.NewHTTPTool()); err != nil {
		return fmt.Errorf("注册工具失败: %w", err)
	}

	userID := modelSelectionUserID(opts)
	modelStore, err := storage.OpenModelSelectionStore(parent, cfg.Database.DSN())
	if err != nil {
		fmt.Fprintf(errOut, "模型选择存储不可用，本次 Web 会话将只使用默认模型: %v\n", err)
		modelStore = storage.NoopModelSelectionStore{}
	}
	defer func() {
		if err := modelStore.Close(); err != nil {
			fmt.Fprintf(errOut, "关闭模型选择存储失败: %v\n", err)
		}
	}()

	redisConfig := cfg.RedisSettings()
	memoryTTL, err := shortTermMemoryTTL(redisConfig.ShortTermTTL)
	if err != nil {
		fmt.Fprintf(errOut, "短期记忆 TTL 配置无效，将使用 24h: %v\n", err)
		memoryTTL = 24 * time.Hour
	}
	memoryStore, err := storage.OpenActiveMemoryStore(parent, redisConfig.Addr, redisConfig.Password, redisConfig.DB, memoryTTL)
	if err != nil {
		fmt.Fprintf(errOut, "短期记忆 Redis 不可用，Web 会话将回退到进程内上下文: %v\n", err)
		memoryStore = storage.NoopActiveMemoryStore{}
	}
	defer func() {
		if err := memoryStore.Close(); err != nil {
			fmt.Fprintf(errOut, "关闭短期记忆存储失败: %v\n", err)
		}
	}()

	currentLLMIndex := 0
	if strings.TrimSpace(opts.Model) == "" {
		currentLLMIndex = restoreModelSelection(parent, io.Discard, io.Discard, modelStore, userID, agent.DefaultSessionID, cfg)
	}

	runner := newSessionAgentRunner(
		cfg,
		registry,
		modelStore,
		modelStore,
		memoryStore,
		userID,
		currentLLMIndex,
		strings.TrimSpace(opts.Model) != "",
	)

	chatAgent, err := buildChatAgent(parent, cfg, registry, currentLLMIndex, nil)
	if err != nil {
		return fmt.Errorf("创建 Agent 失败: %w", err)
	}
	runner.replaceHistories(chatAgent.SnapshotHistories())

	handler := web.NewHandler(
		registry,
		web.WithModelName(cfg.LLM[0].DisplayName(0)),
	)
	handler.RegisterAgent(runner)

	router := newRouter(handler, serveOpts)

	addr := normalizeAddr(opts.ServerAddr)
	server := &http.Server{
		Addr:              addr,
		Handler:           router,
		ReadHeaderTimeout: 15 * time.Second,
	}

	fmt.Fprintln(out, "╔══════════════════════════════════════╗")
	fmt.Fprintln(out, "║        Gora Agent Server            ║")
	fmt.Fprintln(out, "╚══════════════════════════════════════╝")
	fmt.Fprintf(out, "🚀 服务监听: http://%s\n", displayAddr(addr))
	fmt.Fprintf(out, "🤖 Agent: %s (模型: %s)\n", runner.ID(), cfg.LLM[currentLLMIndex].DisplayName(currentLLMIndex))
	fmt.Fprintf(out, "🔧 已加载工具: %d 个\n", len(registry.List()))
	if dir := strings.TrimSpace(serveOpts.StaticDir); dir != "" {
		fmt.Fprintf(out, "🗂  静态资源: %s\n", dir)
	} else {
		fmt.Fprintln(out, "🗂  前端入口: 内嵌单文件页面 /")
	}
	fmt.Fprintln(out, "按 Ctrl+C 退出")

	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	errCh := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
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
	if err := server.Shutdown(shutdownCtx); err != nil {
		fmt.Fprintf(errOut, "关闭服务失败: %v\n", err)
	}
	return nil
}

func newRouter(handler *web.Handler, opts serveOptions) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)

	router := gin.New()
	router.Use(gin.Recovery())

	if opts.CORS {
		router.Use(corsMiddleware())
	}

	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "framework": "Gora"})
	})

	api := router.Group("/api")
	{
		api.POST("/chat", handler.HandleChat)
		api.POST("/chat/:agentId", handler.HandleChat)
		api.GET("/agents", handler.HandleListAgents)
		api.GET("/agents/:agentId", handler.HandleGetAgent)
		api.GET("/agents/:agentId/state", handler.HandleGetAgent)
		api.GET("/tools", handler.HandleListTools)
	}

	if dir := strings.TrimSpace(opts.StaticDir); dir != "" {
		mountStatic(router, dir)
	} else {
		mountEmbeddedIndex(router, handler)
	}

	return router
}

func mountEmbeddedIndex(router *gin.Engine, handler *web.Handler) {
	router.GET("/", handler.HandleIndex)
	router.NoRoute(func(c *gin.Context) {
		if c.Request.Method != http.MethodGet {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		handler.HandleIndex(c)
	})
}

func mountStatic(router *gin.Engine, dir string) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}

	indexPath := filepath.Join(abs, "index.html")

	router.NoRoute(func(c *gin.Context) {
		// 仅对 GET 请求做 SPA 兜底，避免误把 API 404 吞掉。
		if c.Request.Method != http.MethodGet {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}

		requested := filepath.Join(abs, cleanRelativePath(c.Request.URL.Path))
		if info, err := os.Stat(requested); err == nil && !info.IsDir() {
			c.File(requested)
			return
		}
		c.File(indexPath)
	})
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.Writer.Header()
		header.Set("Access-Control-Allow-Origin", "*")
		header.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		header.Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
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

func cleanRelativePath(requestPath string) string {
	cleaned := filepath.Clean("/" + requestPath)
	return strings.TrimPrefix(cleaned, string(os.PathSeparator))
}

func init() {
	rootCmd.Flags().StringVar(&serverOptions.StaticDir, "static", "", "覆盖内嵌前端，改为挂载指定静态资源目录")
	rootCmd.Flags().BoolVar(&serverOptions.CORS, "cors", serverOptions.CORS, "是否开启简单 CORS")
}
