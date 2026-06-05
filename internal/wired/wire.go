//go:build wireinject
// +build wireinject

// Package wired 是 Gora 的 wire 注入器汇总点。
//
// 这里只放声明（带 wireinject build tag），实际可调用的 InitInfra 由 wire CLI
// 生成到同包下的 wire_gen.go。运行 `make wire` 重新生成。
package wired

import (
	"context"

	"github.com/google/wire"

	"github.com/Serendipity565/gora/internal/app"
	appconfig "github.com/Serendipity565/gora/internal/config"
	"github.com/Serendipity565/gora/internal/ioc"
)

// InitInfra 装配运行 Gora 所需的基础设施依赖（工具注册表、MySQL、Redis）。
//
// 返回的 cleanup 会按 wire 生成的反序依次释放底层资源；调用方应在错误处理与正常退出
// 时都执行 cleanup（典型用法：拿到后立刻 defer cleanup()）。
func InitInfra(ctx context.Context, cfg appconfig.Config) (*app.Infra, func(), error) {
	wire.Build(
		ioc.ProviderSet,
		wire.Struct(new(app.Infra), "*"),
	)
	return nil, nil, nil
}
