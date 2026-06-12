// Package controller 定义 HTTP 请求处理器，负责解析参数、调用服务层并返回响应。
package controller

import "github.com/google/wire"

// ProviderSet 暴露 controller 包的 wire 构造函数。
var ProviderSet = wire.NewSet(
	NewUser,
	NewAgent,
	NewTool,
	NewModel,
	NewChat,
	NewHistory,
	NewHealth,
)
