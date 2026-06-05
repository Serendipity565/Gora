package sse

import "github.com/google/wire"

// ProviderSet 暴露 sse 包的 wire 构造函数。
//
// SSEWriter 与单次请求绑定，不适合作为进程级 wire 依赖；
// 这里留空 ProviderSet 占位，便于未来需要时统一从 server.go 聚合。
var ProviderSet = wire.NewSet()
