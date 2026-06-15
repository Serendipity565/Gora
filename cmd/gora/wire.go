//go:build wireinject
// +build wireinject

package main

import (
	"context"

	"github.com/Serendipity565/gora/configs"
	"github.com/google/wire"

	"github.com/Serendipity565/gora/internal/agent/tool"
	"github.com/Serendipity565/gora/internal/agent/tool/builtin"
	"github.com/Serendipity565/gora/internal/controller"
	"github.com/Serendipity565/gora/internal/ioc"
	"github.com/Serendipity565/gora/internal/middleware"
	"github.com/Serendipity565/gora/internal/repository"
	"github.com/Serendipity565/gora/internal/repository/redis"
	"github.com/Serendipity565/gora/internal/router"
	"github.com/Serendipity565/gora/internal/server"
	"github.com/Serendipity565/gora/pkg/ijwt"
)

// provideToolRegistry 创建一个内置工具注册表，并把所有内置工具注册进去。
func provideToolRegistry() (*tool.Registry, error) {
	r := tool.NewRegistry()
	for _, t := range []tool.Tool{
		builtin.NewHTTPTool(),
		builtin.NewCalculatorTool(),
		builtin.NewCurrentTimeTool(),
		builtin.NewWebSearchTool(),
	} {
		if err := r.Register(t); err != nil {
			return nil, err
		}
	}
	return r, nil
}

// provideActiveMemoryCache 根据 cfg.Database.Redis 创建短期记忆缓存；
// 未配置 Redis 时退化为 NoopActiveMemoryCache（即不缓存，由数据库历史兜底）。
func provideActiveMemoryCache(ctx context.Context, cfg configs.Config) (redis.ActiveMemoryCache, func(), error) {
	rs := cfg.Database.Redis
	store, err := redis.OpenActiveMemoryCache(ctx, rs.Addr, rs.Password, rs.DB, 0)
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() { _ = store.Close() }
	return store, cleanup, nil
}

// provideChatOptions 返回空的 ChatHandler 选项切片。
// controller.NewChat 的可选项目前没有从配置注入的需求；保留 hook 方便未来扩展。
func provideChatOptions() []controller.ChatOption {
	return nil
}

// newApp 装配运行 Gora 所需的进程级依赖（基础设施 + middleware + 各业务 service / handler +
// 路由引擎），打包成 *App 交给 main.go 启动。
//
// 修改任何 ProviderSet 后必须执行 `make wire` 重新生成 cmd/gora/wire_gen.go。
func newApp(ctx context.Context, cfg configs.Config) (*App, func(), error) {
	wire.Build(
		// 从 cfg 拆出各子配置（统一值类型）。
		configs.ProviderSet,

		// 基础设施
		ioc.ProviderSet,
		ijwt.NewJWT,
		provideToolRegistry,
		provideActiveMemoryCache,
		provideChatOptions,

		// repository / server / controller / router / middleware
		repository.ProviderSet,
		server.ProviderSet,
		controller.ProviderSet,
		router.ProviderSet,
		middleware.ProviderSet,

		// 装配 App
		wire.Struct(new(App), "*"),
	)
	return nil, nil, nil
}
