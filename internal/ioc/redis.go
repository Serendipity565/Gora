package ioc

import (
	"context"
	"fmt"

	"github.com/Serendipity565/gora/internal/config"
	"github.com/go-redis/redis/v8"
)

func InitRedis(conf config.RedisConfig) *redis.Client {
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
