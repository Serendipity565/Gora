// Package repository 是数据访问层的聚合入口。
package repository

import (
	"github.com/Serendipity565/gora/internal/repository/cache"
	"github.com/Serendipity565/gora/internal/repository/dao"
	"github.com/google/wire"
)

// ProviderSet 聚合 repository 层的 wire provider。
var ProviderSet = wire.NewSet(
	dao.NewUserDAO,
)

// 对外暴露的类型别名，避免上层逐一 import 子包。
type (
	ActiveMemoryStore     = cache.ActiveMemoryCache
	NoopActiveMemoryStore = cache.NoopActiveMemoryCache

	UserDAO = dao.UserDAO
)

var (
	ByEmail = dao.ByEmail
	ByID    = dao.ByID
)
