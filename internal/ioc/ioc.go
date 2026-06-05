// Package ioc 收集所有"基础设施初始化"的 wire provider。
//
// 这一层只关心"如何把 cfg 变成 *gorm.DB / *redis.Client / *tool.Registry / *appconfig.Config"，
// 不感知业务实体（Agent、Runner、HTTP handler）。其它 internal 子包通过自己的
// ProviderSet 暴露 New* 构造函数；ioc.ProviderSet 聚合这些基础设施 provider，
// 由 cmd/gora/wire.go 中的注入器一次性 wire.Build。
package ioc

import (
	"github.com/google/wire"
)

// ProviderSet 暴露所有基础设施的初始化函数。
//
// 注意：本 set 只产出"无状态/进程级"的依赖；与 cli 运行时参数（addr、llmIndex 等）
// 相关的部分留给具体注入器在 wire.Build 时叠加。
var ProviderSet = wire.NewSet(
	NewMySQL,
	NewRedis,
	NewToolRegistry,
)
