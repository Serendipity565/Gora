package middleware

import "github.com/google/wire"

// ProviderSet 暴露 middleware 包的 wire 构造函数。
//
// 当前所有中间件都是无依赖的纯函数；新增带依赖的中间件时把对应 New* 加进来。
var ProviderSet = wire.NewSet(
	Cors,
)
