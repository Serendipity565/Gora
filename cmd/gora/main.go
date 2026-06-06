package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/Serendipity565/gora/internal/app"
	appconfig "github.com/Serendipity565/gora/internal/config"
)

// 命令行 flag —— 全部最简化：
//
//	--config  YAML 配置文件路径
//	--addr    HTTP 监听地址
//	--cors    是否启用跨域中间件（前端独立启动需要）
//
// 其它运行时覆盖通过环境变量（DEEPSEEK_API_KEY / GORA_DATABASE_URL / GORA_REDIS_*
// 等）由 app.ApplyEnvOverrides 处理；不再支持 --api-key / --base-url / --model 等
// 老的 cli 子命令风格 flag。
var (
	flagConfig = flag.String("config", appconfig.DefaultPath, "Gora YAML 配置文件路径")
	flagAddr   = flag.String("addr", ":8080", "Gin Web 服务监听地址")
	flagCORS   = flag.Bool("cors", true, "是否启用 CORS 中间件（前端跨域需要）")
)

func main() {
	flag.Parse()

	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()

	cfg, err := appconfig.Read(*flagConfig)
	if err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}
	app.ApplyEnvOverrides(&cfg)
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("配置校验失败: %w", err)
	}

	infra, cleanup, err := initInfra(ctx, cfg)
	if err != nil {
		return fmt.Errorf("初始化基础设施失败: %w", err)
	}
	defer cleanup()

	return app.Run(ctx, os.Stdout, os.Stderr, infra, cfg, app.Options{
		ServerAddr: *flagAddr,
		CORS:       *flagCORS,
	})
}
