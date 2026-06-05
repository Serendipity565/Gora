package cache

import "github.com/google/wire"

// ProviderSet 暴露 cache 包的 wire 构造函数。
//
// OpenActiveMemoryCache 在 addr 为空时返回 NoopActiveMemoryCache，
// 因此 wire 可以无脑使用它作为 ActiveMemoryCache 的 provider。
// 与 dao 同理：生产装配通常走 ioc.NewRedis 以获得 cleanup。
var ProviderSet = wire.NewSet(
	OpenActiveMemoryCache,
)
