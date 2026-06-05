package router

import "github.com/google/wire"

// ProviderSet 暴露 router 包的 wire 构造函数。
//
// New 依赖 *handler.Handler + Options。Options 由具体注入器从 cli flag 派生，
// 因此本 ProviderSet 只导出 New；Options 由调用方提供。
var ProviderSet = wire.NewSet(
	New,
)
