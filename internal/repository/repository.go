// Package repository 是数据访问层的聚合入口。
//
// 真实实现拆分在三个子包中：
//   - model/: 数据库模型（User、Session、Message 等），带 GORM 标签。
//   - dao/  : MySQL 持久化（GORM）。提供 UserDAO / SessionDAO / MessageDAO。
//   - cache/: Redis 缓存。提供 ActiveMemoryCache。
//
// 本文件通过类型别名重新导出常用 API，便于上层（cmd / server）以单一入口装配。
package repository

import (
	"github.com/Serendipity565/gora/internal/repository/model"
	"github.com/Serendipity565/gora/internal/repository/mysql"
	"github.com/Serendipity565/gora/internal/repository/redis"
	"github.com/google/wire"
	"gorm.io/gorm"
)

// ProviderSet 聚合 repository 层的 wire provider。
var ProviderSet = wire.NewSet(
	mysql.NewUserDAO,
	mysql.NewSessionDAO,
	mysql.NewMessageDAO,
)

func InitTables(db *gorm.DB) error {
	models := []any{
		&model.User{},
		&model.Session{},
		&model.Message{},
	}
	return db.AutoMigrate(models...)
}

// 领域数据类型（来自 model/）。
type (
	Session = model.Session
	Message = model.Message
)

type (
	ActiveMemoryStore = redis.ActiveMemoryCache

	UserDAO         = mysql.UserDAO
	UserQueryOption = mysql.UserQueryOption

	SessionDAO         = mysql.SessionDAO
	SessionQueryOption = mysql.SessionQueryOption

	MessageDAO         = mysql.MessageDAO
	MessageQueryOption = mysql.MessageQueryOption
)

// 查询条件 helper（来自 mysql/）。
var (
	ByEmail = mysql.ByEmail
	ByID    = mysql.ByID

	BySessionID     = mysql.BySessionID
	BySessionUserID = mysql.BySessionUserID

	AfterID      = mysql.AfterID
	MessageLimit = mysql.MessageLimit
)
