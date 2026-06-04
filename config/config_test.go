package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	t.Parallel()

	path := writeConfig(t, `
llm:
  - api_key: sk-test
    model: deepseek-reasoner
agent:
  id: custom-agent
  max_history_messages: 12
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(cfg.LLM) != 1 {
		t.Fatalf("expected 1 llm config, got %d", len(cfg.LLM))
	}
	if cfg.LLM[0].Provider != "deepseek" {
		t.Fatalf("expected default provider, got %s", cfg.LLM[0].Provider)
	}
	if cfg.LLM[0].BaseURL != "https://api.deepseek.com" {
		t.Fatalf("expected default base url, got %s", cfg.LLM[0].BaseURL)
	}
	if cfg.LLM[0].Model != "deepseek-reasoner" {
		t.Fatalf("unexpected model: %s", cfg.LLM[0].Model)
	}
	if cfg.Agent.ID != "custom-agent" {
		t.Fatalf("unexpected agent id: %s", cfg.Agent.ID)
	}
	if cfg.Agent.MaxHistoryMessages != 12 {
		t.Fatalf("unexpected max history messages: %d", cfg.Agent.MaxHistoryMessages)
	}
	if cfg.Agent.MaxStreamChunkRunes != 64 {
		t.Fatalf("expected default stream chunk size, got %d", cfg.Agent.MaxStreamChunkRunes)
	}
}

func TestLoadSupportsOpenAIProviderDefaults(t *testing.T) {
	t.Parallel()

	path := writeConfig(t, `
llm:
  - provider: openai
    api_key: sk-openai
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.LLM[0].Provider != "openai" {
		t.Fatalf("unexpected provider: %s", cfg.LLM[0].Provider)
	}
	if cfg.LLM[0].BaseURL != "https://api.openai.com/v1" {
		t.Fatalf("unexpected base url: %s", cfg.LLM[0].BaseURL)
	}
	if cfg.LLM[0].Model != "gpt-4o-mini" {
		t.Fatalf("unexpected model: %s", cfg.LLM[0].Model)
	}
}

func TestLoadRequiresAPIKey(t *testing.T) {
	t.Parallel()

	path := writeConfig(t, `
llm:
  - model: deepseek-chat
`)

	if _, err := Load(path); err == nil {
		t.Fatal("expected empty api key to fail")
	}
}

func TestReadAllowsMissingAPIKey(t *testing.T) {
	t.Parallel()

	path := writeConfig(t, `
llm:
  - model: deepseek-chat
`)

	cfg, err := Read(path)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if cfg.LLM[0].APIKey != "" {
		t.Fatalf("expected empty api key, got %q", cfg.LLM[0].APIKey)
	}
}

func TestReadTrimsDatabaseURL(t *testing.T) {
	t.Parallel()

	path := writeConfig(t, `
llm:
  - model: deepseek-chat
database:
  url: "  gora:gora_dev_password@tcp(localhost:3306)/gora?charset=utf8mb4&parseTime=True&loc=Local  "
`)

	cfg, err := Read(path)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	want := "gora:gora_dev_password@tcp(localhost:3306)/gora?charset=utf8mb4&parseTime=True&loc=Local"
	if cfg.Database.URL != want {
		t.Fatalf("unexpected database url: %q", cfg.Database.URL)
	}
}

func TestReadBuildsDatabaseURLFromMySQLConfig(t *testing.T) {
	t.Parallel()

	path := writeConfig(t, `
llm:
  - model: deepseek-chat
database:
  mysql:
    addr: "  localhost:3306  "
    dbname: "  gora  "
    username: "  gora  "
    password: "  gora_dev_password  "
`)

	cfg, err := Read(path)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	want := "gora:gora_dev_password@tcp(localhost:3306)/gora?charset=utf8mb4&parseTime=True&loc=Local"
	if cfg.DatabaseURL() != want {
		t.Fatalf("unexpected database url: %q", cfg.DatabaseURL())
	}
}

func TestReadRedisConfig(t *testing.T) {
	t.Parallel()

	path := writeConfig(t, `
llm:
  - model: deepseek-chat
database:
  redis:
    addr: "  localhost:6379  "
    password: "  secret  "
    db: 2
    short_term_ttl: "  12h  "
`)

	cfg, err := Read(path)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	redis := cfg.RedisSettings()
	if redis.Addr != "localhost:6379" {
		t.Fatalf("unexpected redis addr: %q", redis.Addr)
	}
	if redis.Password != "secret" {
		t.Fatalf("unexpected redis password: %q", redis.Password)
	}
	if redis.DB != 2 {
		t.Fatalf("unexpected redis db: %d", redis.DB)
	}
	if redis.ShortTermTTL != "12h" {
		t.Fatalf("unexpected redis ttl: %q", redis.ShortTermTTL)
	}
}

func TestReadLegacyRedisConfig(t *testing.T) {
	t.Parallel()

	path := writeConfig(t, `
llm:
  - model: deepseek-chat
redis:
  addr: "localhost:6379"
  password: "legacy"
  db: 1
  short_term_ttl: "6h"
`)

	cfg, err := Read(path)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	redis := cfg.RedisSettings()
	if redis.Password != "legacy" {
		t.Fatalf("unexpected redis password: %q", redis.Password)
	}
	if redis.DB != 1 {
		t.Fatalf("unexpected redis db: %d", redis.DB)
	}
	if redis.ShortTermTTL != "6h" {
		t.Fatalf("unexpected redis ttl: %q", redis.ShortTermTTL)
	}
}

func TestReadRedisDefaultTTL(t *testing.T) {
	t.Parallel()

	path := writeConfig(t, `
llm:
  - model: deepseek-chat
redis:
  addr: "localhost:6379"
`)

	cfg, err := Read(path)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if cfg.RedisSettings().ShortTermTTL != "24h" {
		t.Fatalf("unexpected redis default ttl: %q", cfg.RedisSettings().ShortTermTTL)
	}
}

func TestLoadRejectsUnsupportedProvider(t *testing.T) {
	t.Parallel()

	path := writeConfig(t, `
llm:
  - provider: other
    api_key: sk-test
`)

	if _, err := Load(path); err == nil {
		t.Fatal("expected unsupported provider to fail")
	}
}

func TestFindLLM(t *testing.T) {
	t.Parallel()

	cfg := Config{
		LLM: []LLMConfig{
			{Name: "chat", Provider: "deepseek", Model: "deepseek-chat", APIKey: "sk-1"},
			{Name: "reasoner", Provider: "deepseek", Model: "deepseek-reasoner", APIKey: "sk-2"},
		},
	}

	if _, llm, err := cfg.FindLLM("2"); err != nil || llm.Model != "deepseek-reasoner" {
		t.Fatalf("find by index failed: %v, %#v", err, llm)
	}
	if _, llm, err := cfg.FindLLM("chat"); err != nil || llm.Model != "deepseek-chat" {
		t.Fatalf("find by name failed: %v, %#v", err, llm)
	}
	if _, llm, err := cfg.FindLLM("deepseek-reasoner"); err != nil || llm.Name != "reasoner" {
		t.Fatalf("find by model failed: %v, %#v", err, llm)
	}
}

func TestValidateRejectsDuplicateLLMNames(t *testing.T) {
	t.Parallel()

	cfg := Config{
		LLM: []LLMConfig{
			{Name: "chat", Provider: "deepseek", Model: "deepseek-chat", APIKey: "sk-1"},
			{Name: "CHAT", Provider: "deepseek", Model: "deepseek-reasoner", APIKey: "sk-2"},
		},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected duplicate llm names to fail")
	}
}

func writeConfig(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}
