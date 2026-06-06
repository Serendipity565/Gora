package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Serendipity565/gora/internal/agent/eino"
	appconfig "github.com/Serendipity565/gora/internal/config"
	"github.com/Serendipity565/gora/internal/server"
)

// Run 启动 Gora HTTP 服务并阻塞直到 SIGINT/SIGTERM。
//
// 调用方负责：
//  1. 读 cfg + 跑 ApplyEnvOverrides + cfg.Validate；
//  2. 通过 wire 注入器拿到 *Infra（同时拿到 cleanup，由调用方 defer cleanup()）；
//  3. 调用 Run(ctx, out, errOut, infra, cfg, opts)。
//
// Run 不再处理 cli flag、不再 Open MySQL/Redis；那些事在 main 与 wire 各自完成。
func Run(parent context.Context, out, errOut io.Writer, infra *Infra, cfg appconfig.Config, opts Options) error {
	if parent == nil {
		parent = context.Background()
	}

	registry := infra.Registry
	modelStore := infra.DB
	memoryStore := infra.Cache
	userID := ModelSelectionUserID()

	currentLLMIndex := restoreModelSelection(parent, io.Discard, io.Discard, modelStore, userID, eino.DefaultSessionID, cfg)

	runner := NewRunner(
		cfg,
		registry,
		modelStore, // ModelSelectionStore
		modelStore, // ChatHistoryStore（同一个 GORM 复合 store）
		memoryStore,
		userID,
		currentLLMIndex,
		false, // forceModel：cli flag 已移除，保留 false 给未来扩展
	)

	chatAgent, err := buildChatAgent(parent, cfg, registry, currentLLMIndex, nil)
	if err != nil {
		return fmt.Errorf("创建 Agent 失败: %w", err)
	}
	runner.PrimeHistories(chatAgent.SnapshotHistories())

	handler := server.NewHandler(
		registry,
		server.WithModelName(cfg.LLM[0].Name),
		server.WithModelSelector(runner),
	)
	handler.RegisterAgent(runner)

	router := server.NewRouter(handler, server.RouterOptions{CORS: opts.CORS})

	addr := normalizeAddr(opts.ServerAddr)
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           router,
		ReadHeaderTimeout: 15 * time.Second,
	}

	fmt.Fprintf(out, "🚀 服务监听: http://%s\n", displayAddr(addr))
	fmt.Fprintf(out, "🤖 Agent: %s (模型: %s)\n", runner.ID(), cfg.LLM[currentLLMIndex].Name)
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
