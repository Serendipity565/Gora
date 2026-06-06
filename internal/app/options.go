package app

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Serendipity565/gora/internal/agent/eino"
	appconfig "github.com/Serendipity565/gora/internal/config"
	storage "github.com/Serendipity565/gora/internal/repository"
)

// ApplyEnvOverrides 把环境变量中的覆盖项写入 cfg。
//
// 取消 cli flag 之后，唯一支持运行时覆盖的途径就是环境变量。处理顺序与命名约定：
//
//	DEEPSEEK_API_KEY              -> cfg.LLM[provider=deepseek].APIKey（仅在 cfg 该项为空时写入）
//	OPENAI_API_KEY                -> 同上，provider=openai
//	GORA_DATABASE_URL             -> cfg.Database.URL（仅在 cfg 为空时）
//	GORA_REDIS_ADDR/PASSWORD/DB   -> cfg.Database.Redis.*（同时同步顶层 cfg.Redis 旧字段）
//	GORA_SHORT_TERM_MEMORY_TTL    -> cfg.Database.Redis.ShortTermTTL
//	GORA_USER_ID                  -> 通过 ModelSelectionUserID() 在 Run 中读取
func ApplyEnvOverrides(cfg *appconfig.Config) {
	applyProviderAPIKeyEnv(cfg, "DEEPSEEK_API_KEY", "deepseek")
	applyProviderAPIKeyEnv(cfg, "OPENAI_API_KEY", "openai")

	if envDatabaseURL := strings.TrimSpace(os.Getenv("GORA_DATABASE_URL")); envDatabaseURL != "" && strings.TrimSpace(cfg.Database.URL) == "" {
		cfg.Database.URL = envDatabaseURL
	}
	if envRedisAddr := strings.TrimSpace(os.Getenv("GORA_REDIS_ADDR")); envRedisAddr != "" {
		cfg.Database.Redis.Addr = envRedisAddr
		cfg.Redis.Addr = envRedisAddr
	}
	if envRedisPassword := strings.TrimSpace(os.Getenv("GORA_REDIS_PASSWORD")); envRedisPassword != "" {
		cfg.Database.Redis.Password = envRedisPassword
		cfg.Redis.Password = envRedisPassword
	}
	if envRedisDB := strings.TrimSpace(os.Getenv("GORA_REDIS_DB")); envRedisDB != "" {
		if redisDB, err := strconv.Atoi(envRedisDB); err == nil {
			cfg.Database.Redis.DB = redisDB
			cfg.Redis.DB = redisDB
		}
	}
	if envMemoryTTL := strings.TrimSpace(os.Getenv("GORA_SHORT_TERM_MEMORY_TTL")); envMemoryTTL != "" {
		cfg.Database.Redis.ShortTermTTL = envMemoryTTL
		cfg.Redis.ShortTermTTL = envMemoryTTL
	}
}

func applyProviderAPIKeyEnv(cfg *appconfig.Config, envName, provider string) {
	apiKey := strings.TrimSpace(os.Getenv(envName))
	if apiKey == "" {
		return
	}
	for index := range cfg.LLM {
		if !strings.EqualFold(strings.TrimSpace(cfg.LLM[index].Provider), provider) {
			continue
		}
		if strings.TrimSpace(cfg.LLM[index].APIKey) == "" {
			cfg.LLM[index].APIKey = apiKey
		}
	}
}

// ModelSelectionUserID 返回模型选择持久化使用的用户 ID。
// 优先级：环境变量 GORA_USER_ID > storage.LocalUserID（通常是 "local"）。
func ModelSelectionUserID() string {
	if userID := strings.TrimSpace(os.Getenv("GORA_USER_ID")); userID != "" {
		return userID
	}
	return storage.LocalUserID
}

// shortTermMemoryTTL 解析 cfg.Redis.ShortTermTTL。空 → 24h；非法 → 错误。
func shortTermMemoryTTL(raw string) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 24 * time.Hour, nil
	}
	return time.ParseDuration(raw)
}

// normalizeSessionID 空白 / 空字符串 → eino.DefaultSessionID。
func normalizeSessionID(sessionID string) string {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return eino.DefaultSessionID
	}
	return sessionID
}

// normalizeAddr 把 addr 规整为 "host:port" 形式：
//   - 空 → ":8080"
//   - 不含 ":" → ":<addr>"
//   - 已是合法形式则原样返回
func normalizeAddr(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return ":8080"
	}
	if !strings.Contains(addr, ":") {
		return ":" + addr
	}
	return addr
}

// displayAddr 把 ":8080" 这种监听地址展示成 "localhost:8080"，便于用户点开。
func displayAddr(addr string) string {
	if strings.HasPrefix(addr, ":") {
		return "localhost" + addr
	}
	return addr
}
