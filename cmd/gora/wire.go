//go:build wireinject
// +build wireinject

package main

import (
	"context"

	"github.com/google/wire"

	"github.com/Serendipity565/gora/internal/app"
	appconfig "github.com/Serendipity565/gora/internal/config"
	"github.com/Serendipity565/gora/internal/ioc"
)

// initInfra 装配运行 Gora 所需的基础设施依赖（工具注册表、MySQL、Redis）。
//
// 与 main.go 同包（kratos 风格）：cmd/gora/main.go 直接调用，再把 *app.Infra 交给 app.Run。
// 修改任何 ProviderSet 后必须执行 `make wire` 重新生成 cmd/gora/wire_gen.go。
//
// 返回的 cleanup 会按 wire 生成的反序依次释放底层资源；调用方应在拿到后立即 defer。
func initInfra(ctx context.Context, cfg appconfig.Config) (*app.Infra, func(), error) {
	wire.Build(
		ioc.ProviderSet,
		wire.Struct(new(app.Infra), "*"),
	)
	return nil, nil, nil
}
