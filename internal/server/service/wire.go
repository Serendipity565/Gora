package service

import "github.com/google/wire"

// ProviderSet 暴露 service 包的 wire 构造函数。
var ProviderSet = wire.NewSet(
	NewChatService,
)
