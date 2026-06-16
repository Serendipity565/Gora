// Package ioc 收集所有"基础设施初始化"的 wire provider。
//
// 这一层只关心"如何把 cfg 变成 *gorm.DB / *redis.Client 等
// 不感知业务实体（Agent、Runner、HTTP controller）。其它 internal 子包通过自己的
// ProviderSet 暴露 New* 构造函数；ioc.ProviderSet 聚合这些基础设施 provider，
// 由 cmd/gora/wire.go 中的注入器一次性 wire.Build。
package ioc

import (
	"github.com/google/wire"
)

// ProviderSet 暴露所有基础设施的初始化函数。 。
var ProviderSet = wire.NewSet(
	InitLogger,
	InitMysql,
	InitRedis,
)
