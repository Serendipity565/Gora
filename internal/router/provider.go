package router

import "github.com/google/wire"

// ProviderSet 暴露 router 包的 wire 构造函数。
var ProviderSet = wire.NewSet(
	NewEngine,
)
