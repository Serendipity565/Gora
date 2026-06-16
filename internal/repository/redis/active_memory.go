package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	goredis "github.com/go-redis/redis/v8"

	"github.com/Serendipity565/gora/internal/repository/model"
)

const (
	defaultActiveMemoryTTL = 24 * time.Hour
)

// ActiveMemoryCache 是 (user, agent, session) 维度的短期记忆存储抽象。
type ActiveMemoryCache interface {
	Get(ctx context.Context, userID uint64, agentID, sessionID string) ([]model.Message, bool, error)
	Save(ctx context.Context, userID uint64, agentID, sessionID string, messages []model.Message) error
	Close() error
}

// NoopActiveMemoryCache 在未配置 Redis 时使用，所有操作均无副作用。
type NoopActiveMemoryCache struct{}

func (NoopActiveMemoryCache) Get(context.Context, uint64, string, string) ([]model.Message, bool, error) {
	return nil, false, nil
}

func (NoopActiveMemoryCache) Save(context.Context, uint64, string, string, []model.Message) error {
	return nil
}

func (NoopActiveMemoryCache) Close() error { return nil }

// OpenActiveMemoryCache 根据配置打开 Redis；addr 为空时退化为 Noop。
func OpenActiveMemoryCache(ctx context.Context, addr, password string, db int, ttl time.Duration) (ActiveMemoryCache, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return NoopActiveMemoryCache{}, nil
	}
	if ttl <= 0 {
		ttl = defaultActiveMemoryTTL
	}

	client := goredis.NewClient(&goredis.Options{
		Addr:     addr,
		Password: strings.TrimSpace(password),
		DB:       db,
	})
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("ping redis: %w", err)
	}
	return &RedisActiveMemoryCache{
		client: client,
		ttl:    ttl,
	}, nil
}

// RedisActiveMemoryCache 是基于 go-redis 的实现。
type RedisActiveMemoryCache struct {
	client *goredis.Client
	ttl    time.Duration
}

func (s *RedisActiveMemoryCache) Get(ctx context.Context, userID uint64, agentID, sessionID string) ([]model.Message, bool, error) {
	key, err := activeMemoryKey(userID, agentID, sessionID)
	if err != nil {
		return nil, false, err
	}

	payload, err := s.client.Get(ctx, key).Bytes()
	if errors.Is(err, goredis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("get active memory: %w", err)
	}

	var snapshot activeMemorySnapshot
	if err := json.Unmarshal(payload, &snapshot); err != nil {
		return nil, false, fmt.Errorf("decode active memory: %w", err)
	}
	return snapshot.Messages, true, nil
}

func (s *RedisActiveMemoryCache) Save(ctx context.Context, userID uint64, agentID, sessionID string, messages []model.Message) error {
	key, err := activeMemoryKey(userID, agentID, sessionID)
	if err != nil {
		return err
	}

	snapshot := activeMemorySnapshot{
		Messages: messages,
		SavedAt:  time.Now(),
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("encode active memory: %w", err)
	}
	if err := s.client.Set(ctx, key, payload, s.ttl).Err(); err != nil {
		return fmt.Errorf("save active memory: %w", err)
	}
	return nil
}

func (s *RedisActiveMemoryCache) Close() error {
	return s.client.Close()
}

type activeMemorySnapshot struct {
	Messages []model.Message `json:"messages"`
	SavedAt  time.Time       `json:"saved_at"`
}

func activeMemoryKey(userID uint64, agentID, sessionID string) (string, error) {
	agentID = strings.TrimSpace(agentID)
	sessionID = strings.TrimSpace(sessionID)
	if agentID == "" {
		return "", fmt.Errorf("agent id cannot be empty")
	}
	if sessionID == "" {
		return "", fmt.Errorf("session id cannot be empty")
	}
	return fmt.Sprintf("gora:active-memory:%d:%s:%s", userID, agentID, sessionID), nil
}
