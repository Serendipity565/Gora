package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	redis "github.com/go-redis/redis/v8"
)

const defaultActiveMemoryTTL = 24 * time.Hour

type ActiveMemoryStore interface {
	Get(ctx context.Context, userID, agentID, sessionID string) ([]ChatMessage, bool, error)
	Save(ctx context.Context, userID, agentID, sessionID string, messages []ChatMessage) error
	Close() error
}

type NoopActiveMemoryStore struct{}

func OpenActiveMemoryStore(ctx context.Context, addr, password string, db int, ttl time.Duration) (ActiveMemoryStore, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return NoopActiveMemoryStore{}, nil
	}
	if ttl <= 0 {
		ttl = defaultActiveMemoryTTL
	}

	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: strings.TrimSpace(password),
		DB:       db,
	})
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("ping redis: %w", err)
	}
	return &RedisActiveMemoryStore{
		client: client,
		ttl:    ttl,
	}, nil
}

func (NoopActiveMemoryStore) Get(context.Context, string, string, string) ([]ChatMessage, bool, error) {
	return nil, false, nil
}

func (NoopActiveMemoryStore) Save(context.Context, string, string, string, []ChatMessage) error {
	return nil
}

func (NoopActiveMemoryStore) Close() error {
	return nil
}

type RedisActiveMemoryStore struct {
	client *redis.Client
	ttl    time.Duration
}

func (s *RedisActiveMemoryStore) Get(ctx context.Context, userID, agentID, sessionID string) ([]ChatMessage, bool, error) {
	key, err := activeMemoryKey(userID, agentID, sessionID)
	if err != nil {
		return nil, false, err
	}

	payload, err := s.client.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
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

func (s *RedisActiveMemoryStore) Save(ctx context.Context, userID, agentID, sessionID string, messages []ChatMessage) error {
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

func (s *RedisActiveMemoryStore) Close() error {
	return s.client.Close()
}

type activeMemorySnapshot struct {
	Messages []ChatMessage `json:"messages"`
	SavedAt  time.Time     `json:"saved_at"`
}

func activeMemoryKey(userID, agentID, sessionID string) (string, error) {
	userID = normalizeKey(userID, LocalUserID)
	agentID = strings.TrimSpace(agentID)
	sessionID = strings.TrimSpace(sessionID)
	if agentID == "" {
		return "", fmt.Errorf("agent id cannot be empty")
	}
	if sessionID == "" {
		return "", fmt.Errorf("session id cannot be empty")
	}
	return fmt.Sprintf("gora:active-memory:%s:%s:%s", userID, agentID, sessionID), nil
}
