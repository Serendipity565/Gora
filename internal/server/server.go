// Package server 是 Gora 后端 HTTP/SSE 服务的顶层装配点。
//
// 子包：
//   - api/v1/{request,response}: API DTO 定义。
//   - handler/                 : 具体请求处理（controller 等价物）。
//   - middleware/              : Gin 中间件。
//   - router/                  : 路由注册。
//   - sse/                     : Server-Sent Events 写入器。
//   - service/                 : 业务编排层（chat 流式编排等）。
//
// 类型别名提供与原 web 包接近的便捷入口，方便上层（cli / app）以单一 import 完成装配。
package server

import (
	"github.com/Serendipity565/gora/internal/server/api/v1/response"
	"github.com/Serendipity565/gora/internal/server/handler"
	"github.com/Serendipity565/gora/internal/server/router"
	"github.com/Serendipity565/gora/internal/server/sse"
)

// Handler 是请求处理器类型，由 handler.New 创建。
type Handler = handler.Handler

// HandlerOption 是 Handler 的构造选项。
type HandlerOption = handler.Option

// AgentRunner / SessionRunner / ModelSelector 是 Handler 依赖的接口契约。
type (
	AgentRunner   = handler.AgentRunner
	SessionRunner = handler.SessionRunner
	ModelSelector = handler.ModelSelector
)

// 响应 DTO 别名（来自 api/v1/response/）。
type (
	AgentInfo = response.AgentInfo
	ToolInfo  = response.ToolInfo
	ModelInfo = response.ModelInfo
)

// SSEWriter 是 Server-Sent Events 写入器（来自 sse/）。
type SSEWriter = sse.SSEWriter

// RouterOptions 是 router.New 的可选参数（CORS 等）。
type RouterOptions = router.Options

// NewHandler 创建 Handler，等价于 handler.New。
var NewHandler = handler.New

// WithModelName 设置 Handler 的默认模型名（仅用于元数据）。
var WithModelName = handler.WithModelName

// WithChatTimeout 设置 Handler 的 Chat 超时。
var WithChatTimeout = handler.WithChatTimeout

// WithModelSelector 注入 ModelSelector，启用 /api/models 系列接口。
var WithModelSelector = handler.WithModelSelector

// NewSSEWriter 创建 SSE 写入器。
var NewSSEWriter = sse.NewSSEWriter

// NewRouter 装配并返回一个配置好的 *gin.Engine。
var NewRouter = router.New
