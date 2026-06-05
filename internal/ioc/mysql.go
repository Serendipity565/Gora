package ioc

import (
	"context"

	appconfig "github.com/Serendipity565/gora/internal/config"
	"github.com/Serendipity565/gora/internal/repository/dao"
)

// NewMySQL 根据 cfg.Database.DSN() 打开 MySQL 连接。
//
// 当 DSN 为空（典型场景：本地纯 CLI 测试，没起 docker-compose）时退化为 NoopDatabaseStore，
// 不会让程序启动失败。返回的 cleanup 关闭底层 sql.DB；wire 会在注入器尾部串好整个 cleanup 链。
func NewMySQL(ctx context.Context, cfg appconfig.Config) (dao.DatabaseStore, func(), error) {
	store, err := dao.OpenDatabaseStore(ctx, cfg.Database.DSN())
	if err != nil {
		// MySQL 连不上不阻塞启动：上层在装配过程中读到 noop store 即可继续，
		// 但仍把错误抛给注入器，让 wire 的 cleanup 不会被 silently 触发。
		return dao.NoopDatabaseStore{}, func() {}, nil
	}
	cleanup := func() {
		_ = store.Close()
	}
	return store, cleanup, nil
}
