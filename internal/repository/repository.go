// Package repository 是数据访问层的聚合入口。
//
// 真实实现拆分在三个子包中：
//   - model/: 领域数据类型（ChatMessage、ModelSelection），不带 GORM/Redis 标签。
//   - dao/  : MySQL 持久化（GORM）。提供 ModelSelectionDAO / ChatHistoryDAO / DatabaseStore。
//   - cache/: Redis 缓存。提供 ActiveMemoryCache。
//
// 本文件通过类型别名重新导出常用 API，便于上层（cli / server）以单一入口装配，
// 同时保留与既有调用点（原 storage 包）一致的语义，降低迁移成本。
package repository

import (
	"context"
	"time"

	"github.com/Serendipity565/gora/internal/repository/cache"
	"github.com/Serendipity565/gora/internal/repository/dao"
	"github.com/Serendipity565/gora/internal/repository/model"
)

// LocalUserID 是缺省 user_id。
const LocalUserID = dao.LocalUserID

// 领域数据类型（来自 model/）。
type (
	ChatMessage    = model.ChatMessage
	ModelSelection = model.ModelSelection
)

// 持久化抽象（来自 dao/）。
type (
	ModelSelectionStore = dao.ModelSelectionDAO
	ChatHistoryStore    = dao.ChatHistoryDAO
	DatabaseStore       = dao.DatabaseStore
	NoopDatabaseStore   = dao.NoopDatabaseStore

	// NoopModelSelectionStore 是历史命名，等价于 NoopDatabaseStore。
	NoopModelSelectionStore = dao.NoopDatabaseStore
)

// 短期记忆缓存抽象（来自 cache/）。
type (
	ActiveMemoryStore     = cache.ActiveMemoryCache
	NoopActiveMemoryStore = cache.NoopActiveMemoryCache
)

// OpenDatabaseStore 等价于 dao.OpenDatabaseStore。
func OpenDatabaseStore(ctx context.Context, databaseURL string) (DatabaseStore, error) {
	return dao.OpenDatabaseStore(ctx, databaseURL)
}

// OpenModelSelectionStore 是历史命名，等价于 OpenDatabaseStore，保留以兼容现有调用点。
func OpenModelSelectionStore(ctx context.Context, databaseURL string) (DatabaseStore, error) {
	return dao.OpenDatabaseStore(ctx, databaseURL)
}

// OpenActiveMemoryStore 等价于 cache.OpenActiveMemoryCache。
func OpenActiveMemoryStore(ctx context.Context, addr, password string, db int, ttl time.Duration) (ActiveMemoryStore, error) {
	return cache.OpenActiveMemoryCache(ctx, addr, password, db, ttl)
}
