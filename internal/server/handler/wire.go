package handler

import "github.com/google/wire"

// ProviderSet 暴露 handler 包的 wire 构造函数。
//
// New 依赖 *tool.Registry + 一组 Option（WithModelName / WithModelSelector），
// Option 通常由调用方根据 cfg 与 runner 在 wire.Build 之外自行组装，
// 因此本 ProviderSet 仅暴露最基础的构造，再由具体注入器补 Option。
var ProviderSet = wire.NewSet(
	New,
)
