package dao

import "github.com/google/wire"

// ProviderSet 暴露 dao 包的 wire 构造函数。
//
// OpenDatabaseStore 在 cfg.Database.DSN() 为空时返回 NoopDatabaseStore；
// 因此本 set 直接用 OpenDatabaseStore 作为 DatabaseStore 的 provider。
// 注意：实际产线中 ioc.NewMySQL 已经做了同样的事情，多数注入器会用 ioc 版本以获得 cleanup。
var ProviderSet = wire.NewSet(
	OpenDatabaseStore,
)
