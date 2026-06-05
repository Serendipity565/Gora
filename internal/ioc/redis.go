package ioc

import (
	"context"
	"strings"
	"time"

	appconfig "github.com/Serendipity565/gora/internal/config"
	"github.com/Serendipity565/gora/internal/repository/cache"
)

// defaultShortTermMemoryTTL 与原 cli 实现保持一致，避免回归。
const defaultShortTermMemoryTTL = 24 * time.Hour

// NewRedis 根据 cfg.Database.Redis 打开 Redis 客户端。
//
// 当 addr 为空时退化为 NoopActiveMemoryCache。
// shortTermTTL 配置无效时回落到 24h（不阻塞启动）。
func NewRedis(ctx context.Context, cfg appconfig.Config) (cache.ActiveMemoryCache, func(), error) {
	redisCfg := cfg.RedisSettings()
	ttl := parseShortTermTTL(redisCfg.ShortTermTTL)

	store, err := cache.OpenActiveMemoryCache(ctx, redisCfg.Addr, redisCfg.Password, redisCfg.DB, ttl)
	if err != nil {
		// Redis 连不上不阻塞启动；返回 noop 让上层继续。
		return cache.NoopActiveMemoryCache{}, func() {}, nil
	}
	cleanup := func() {
		_ = store.Close()
	}
	return store, cleanup, nil
}

func parseShortTermTTL(raw string) time.Duration {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultShortTermMemoryTTL
	}
	if d, err := time.ParseDuration(raw); err == nil {
		return d
	}
	return defaultShortTermMemoryTTL
}
