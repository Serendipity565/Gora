package ioc

import (
	"context"
	"fmt"

	"github.com/Serendipity565/gora/configs"
	"github.com/go-redis/redis/v8"
)

// InitRedis 初始化 Redis 客户端并验证连接。
// 连接失败时直接 panic，因为 Redis 是应用运行的必要依赖。
func InitRedis(conf configs.RedisConfig) *redis.Client {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{
		Addr:     conf.Addr,
		Password: conf.Password,
		DB:       conf.DB,
	})
	if err := rdb.Ping(ctx).Err(); err != nil {
		panic(fmt.Sprintf("Redis 连接失败: %v", err))
	}
	return rdb
}
