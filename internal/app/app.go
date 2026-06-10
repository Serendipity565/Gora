// Package app 是 Gora 应用的装配 + 启动层。
//
// 不再有 cli 子命令包：Cobra 已被移除，main.go 直接调用 wire 生成的注入器
// 拿到 *Infra，然后调用 app.Run 启动 HTTP 服务。
//
// 子文件职责：
//   - app.go      — Infra 聚合类型 + Options（运行参数）。
//   - options.go  — 环境变量覆写 / 各种 normalize 工具。
//   - runner.go   — Runner（多会话 Agent 运行器，实现 server.SessionRunner / ModelSelector）。
//   - chat.go     — Agent 构建、模型选择持久化、历史与短期记忆读写助手。
//   - server.go   — Run(parent, out, errOut, infra, cfg, opts)：HTTP 服务器生命周期。
package app

import (
	"github.com/Serendipity565/gora/internal/agent/tool"
	"github.com/Serendipity565/gora/internal/repository/cache"
	"github.com/Serendipity565/gora/internal/repository/dao"
	"github.com/Serendipity565/gora/internal/server/middleware"
	"github.com/Serendipity565/gora/pkg/ijwt"
	"github.com/Serendipity565/gora/pkg/logger"
)

// Infra 聚合运行 Gora 所需的进程级基础设施依赖。
//
// 由 cmd/gora/wire.go 中的 InitInfra 装配；与 cli flag 无关，flag 派生的运行时参数
// 由 Options + Run 自行处理。
type Infra struct {
	Registry    *tool.Registry
	DB          dao.DatabaseStore
	Cache       cache.ActiveMemoryCache
	Logger      logger.Logger
	JWT         *ijwt.JWT
	Middlewares *middleware.Bundle
}

// Options 控制 Run 的可调行为。当前只有两项；将来如需更多运行时开关，
// 在这里扩展即可，main.go 通过 stdlib flag 解析后传入。
type Options struct {
	// ServerAddr 是 HTTP 监听地址，例如 ":8080"。空字符串会回落到 ":8080"。
	ServerAddr string
	// CORS 控制是否启用简易 CORS 中间件，默认建议 true（前端独立 dev server 需要跨域）。
	CORS bool
}
