// Command gora 是 Gora 后端服务的入口。
//
// 启动流程：
//  1. 读取 yaml 配置 + 校验；
//  2. 通过 wire 注入器拿到 *App（含基础设施 / 各业务 service / handler / 路由引擎）；
//  3. 把 Runner 注册到 AgentService / ModelService；
//  4. 启动期 seed admin 账号（不存在时建）；
//  5. 起 HTTP server，阻塞直到 SIGINT/SIGTERM。
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Serendipity565/gora/configs"
	"github.com/gin-gonic/gin"

	"github.com/Serendipity565/gora/internal/agent/runner"
	"github.com/Serendipity565/gora/internal/agent/tool"
	"github.com/Serendipity565/gora/internal/domain"
	"github.com/Serendipity565/gora/internal/repository"
	"github.com/Serendipity565/gora/internal/server"
	"github.com/Serendipity565/gora/pkg/ijwt"
	"github.com/Serendipity565/gora/pkg/logger"
)

// App 聚合 wire 装配出来的进程级依赖。它由 newApp 产出（参见 wire.go），
// 字段都用 wire 标签 "*" 自动填充，main.go 只读不写。
type App struct {
	Registry   *tool.Registry
	SessionDAO repository.SessionDAO
	MessageDAO repository.MessageDAO
	Cache      repository.ActiveMemoryStore
	Logger     logger.Logger
	JWT        *ijwt.JWT

	AgentService server.AgentService
	ModelService server.ModelService
	UserService  server.UserService

	Router *gin.Engine
}

var flagConfig = flag.String("config", configs.DefaultPath, "Gora YAML 配置文件路径")

func main() {
	flag.Parse()

	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()

	cfg, err := configs.Read(*flagConfig)
	if err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("配置校验失败: %w", err)
	}

	app, cleanup, err := newApp(ctx, cfg)
	if err != nil {
		return fmt.Errorf("初始化应用失败: %w", err)
	}
	defer cleanup()

	// Runner 依赖 cfg + 各 store，无法 wire；这里手动 New 后注入到 service。
	r := runner.New(cfg, app.Registry, app.SessionDAO, app.MessageDAO, app.Cache, 0)
	chatAgent, err := r.BuildBootstrapAgent(ctx)
	if err != nil {
		return fmt.Errorf("创建 Agent 失败: %w", err)
	}
	r.PrimeHistories(chatAgent.SnapshotHistories())

	app.AgentService.Register(r)
	app.AgentService.SetModel(cfg.LLM[r.LLMIndex()].Name)
	app.ModelService.SetSelector(r)

	// 启动期 seed：插入 yaml 中配置的 admin 账号（不存在则建，存在则跳过）。
	if err := app.UserService.EnsureAdmin(ctx, &domain.User{
		Email:    cfg.Admin.Email,
		Password: cfg.Admin.Password,
		Username: cfg.Admin.Username,
	}); err != nil {
		return fmt.Errorf("seed admin 账号失败: %w", err)
	}

	addr := normalizeAddr(cfg.Server.Addr)
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           app.Router,
		ReadHeaderTimeout: 15 * time.Second,
	}

	fmt.Fprintf(os.Stdout, "🚀 服务监听: http://%s\n", displayAddr(addr))
	fmt.Fprintf(os.Stdout, "🤖 Agent: %s (模型: %s)\n", r.ID(), cfg.LLM[r.LLMIndex()].Name)
	fmt.Fprintf(os.Stdout, "🔧 已加载工具: %d 个\n", len(app.Registry.List()))
	fmt.Fprintln(os.Stdout, "🗂  纯 API 服务（无内嵌前端）：使用 frontend/ 单独启动 UI，或访问 /api/* 与 /health")
	fmt.Fprintln(os.Stdout, "按 Ctrl+C 退出")

	runCtx, cancel := context.WithCancel(ctx)
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
		fmt.Fprintln(os.Stdout, "\n👋 正在关闭...")
	case <-runCtx.Done():
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("HTTP 服务异常退出: %w", err)
		}
		return nil
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		fmt.Fprintf(os.Stderr, "关闭服务失败: %v\n", err)
	}
	return nil
}

// normalizeAddr 把 addr 规整为 "host:port" 形式：
//   - 空 → ":8080"
//   - 不含 ":" → ":<addr>"
//   - 已是合法形式则原样返回
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

// displayAddr 把 ":8080" 这种监听地址展示成 "localhost:8080"，便于用户点开。
func displayAddr(addr string) string {
	if strings.HasPrefix(addr, ":") {
		return "localhost" + addr
	}
	return addr
}
