// Package app 现在只承担：
//
//  1. 定义 Infra 这个聚合类型，让上层（cli / 测试）只需 import 一个包就拿到全部基础设施依赖；
//  2. 与 chat REPL 共享的 sessionAgentRunner 之外的小工具（暂无）。
//
// 真正的"打开 MySQL / Redis、注册工具"逻辑由 internal/ioc 提供，
// 通过 internal/wired 中 wire.Build 生成的 InitInfra 拼装到 Infra 上。
package app

import (
	"github.com/Serendipity565/gora/internal/agent/tool"
	"github.com/Serendipity565/gora/internal/repository/cache"
	"github.com/Serendipity565/gora/internal/repository/dao"
)

// Infra 聚合运行 Gora 所需的进程级基础设施依赖。
//
// 与 cli 运行时参数（addr / llmIndex / forceModel 等）无关：那些参数由 cli 在拿到
// Infra 之后自行组装到 sessionAgentRunner / Handler 上。
type Infra struct {
	Registry *tool.Registry
	DB       dao.DatabaseStore
	Cache    cache.ActiveMemoryCache
}
