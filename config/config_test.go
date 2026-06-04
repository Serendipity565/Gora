package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad(t *testing.T) {
	t.Parallel()

	path := writeConfig(t, withAgentConfig(`
llm:
  - name: reasoner
    provider: deepseek
    api_key: "  sk-test  "
    base_url: "  https://api.deepseek.com  "
    model: "  deepseek-reasoner  "
database:
  url: "  gora:gora_dev_password@tcp(localhost:3306)/gora?charset=utf8mb4&parseTime=True&loc=Local  "
`))

	cfg := Load(path)

	if len(cfg.LLM) != 1 {
		t.Fatalf("expected 1 llm config, got %d", len(cfg.LLM))
	}
	if cfg.LLM[0].Name != "reasoner" {
		t.Fatalf("unexpected name: %q", cfg.LLM[0].Name)
	}
	if cfg.LLM[0].Provider != "deepseek" {
		t.Fatalf("unexpected provider: %q", cfg.LLM[0].Provider)
	}
	if cfg.LLM[0].APIKey != "sk-test" {
		t.Fatalf("unexpected api key: %q", cfg.LLM[0].APIKey)
	}
	if cfg.LLM[0].BaseURL != "https://api.deepseek.com" {
		t.Fatalf("unexpected base url: %q", cfg.LLM[0].BaseURL)
	}
	if cfg.LLM[0].Model != "deepseek-reasoner" {
		t.Fatalf("unexpected model: %q", cfg.LLM[0].Model)
	}
	if cfg.Agent.ID != "agent-1" {
		t.Fatalf("unexpected agent id: %q", cfg.Agent.ID)
	}
	if cfg.Database.DSN() != "gora:gora_dev_password@tcp(localhost:3306)/gora?charset=utf8mb4&parseTime=True&loc=Local" {
		t.Fatalf("unexpected database dsn: %q", cfg.Database.DSN())
	}
}

func TestLoadSupportsOpenAIProvider(t *testing.T) {
	t.Parallel()

	path := writeConfig(t, withAgentConfig(`
llm:
  - provider: openai
    api_key: sk-openai
    base_url: https://api.openai.com/v1
    model: gpt-4o-mini
`))

	cfg := Load(path)

	if cfg.LLM[0].Provider != "openai" {
		t.Fatalf("unexpected provider: %q", cfg.LLM[0].Provider)
	}
	if cfg.LLM[0].BaseURL != "https://api.openai.com/v1" {
		t.Fatalf("unexpected base url: %q", cfg.LLM[0].BaseURL)
	}
	if cfg.LLM[0].Model != "gpt-4o-mini" {
		t.Fatalf("unexpected model: %q", cfg.LLM[0].Model)
	}
}

func TestLoadSupportsMockProviderWithoutCredentials(t *testing.T) {
	t.Parallel()

	path := writeConfig(t, withAgentConfig(`
llm:
  - name: echo
    provider: mock
    model: echo
`))

	_ = Load(path)
}

func TestLoadRequiresAPIKey(t *testing.T) {
	t.Parallel()

	path := writeConfig(t, withAgentConfig(`
llm:
  - provider: deepseek
    base_url: https://api.deepseek.com
    model: deepseek-chat
`))

	defer expectPanic(t)
	_ = Load(path)
}

func TestLoadRequiresBaseURLForNonMock(t *testing.T) {
	t.Parallel()

	path := writeConfig(t, withAgentConfig(`
llm:
  - provider: deepseek
    api_key: sk-test
    model: deepseek-chat
`))

	defer expectPanic(t)
	_ = Load(path)
}

func TestReadTrimsDatabaseURL(t *testing.T) {
	t.Parallel()

	path := writeConfig(t, withAgentConfig(`
llm:
  - provider: mock
    model: echo
database:
  url: "  gora:gora_dev_password@tcp(localhost:3306)/gora?charset=utf8mb4&parseTime=True&loc=Local  "
`))

	cfg, err := Read(path)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	want := "gora:gora_dev_password@tcp(localhost:3306)/gora?charset=utf8mb4&parseTime=True&loc=Local"
	if cfg.Database.URL != want {
		t.Fatalf("unexpected database url: %q", cfg.Database.URL)
	}
}

func TestReadBuildsDatabaseDSNFromMySQLConfig(t *testing.T) {
	t.Parallel()

	path := writeConfig(t, withAgentConfig(`
llm:
  - provider: mock
    model: echo
database:
  mysql:
    addr: "  localhost:3306  "
    dbname: "  gora  "
    username: "  gora  "
    password: "  gora_dev_password  "
`))

	cfg, err := Read(path)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	want := "gora:gora_dev_password@tcp(localhost:3306)/gora?charset=utf8mb4&parseTime=True&loc=Local"
	if cfg.Database.DSN() != want {
		t.Fatalf("unexpected database dsn: %q", cfg.Database.DSN())
	}
}

func TestReadRedisConfig(t *testing.T) {
	t.Parallel()

	path := writeConfig(t, withAgentConfig(`
llm:
  - provider: mock
    model: echo
database:
  redis:
    addr: "  localhost:6379  "
    password: "  secret  "
    db: 2
    short_term_ttl: "  12h  "
`))

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

	path := writeConfig(t, withAgentConfig(`
llm:
  - provider: mock
    model: echo
redis:
  addr: localhost:6379
  password: legacy
  db: 1
  short_term_ttl: 6h
`))

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

func TestReadLeavesRedisTTLUnsetWhenOmitted(t *testing.T) {
	t.Parallel()

	path := writeConfig(t, withAgentConfig(`
llm:
  - provider: mock
    model: echo
redis:
  addr: localhost:6379
`))

	cfg, err := Read(path)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if cfg.RedisSettings().ShortTermTTL != "" {
		t.Fatalf("unexpected redis ttl: %q", cfg.RedisSettings().ShortTermTTL)
	}
}

func TestFindLLM(t *testing.T) {
	t.Parallel()

	cfg := Config{
		LLM: []LLMConfig{
			{Name: "chat", Provider: "deepseek", Model: "deepseek-chat", APIKey: "sk-1", BaseURL: "https://api.deepseek.com"},
			{Name: "reasoner", Provider: "deepseek", Model: "deepseek-reasoner", APIKey: "sk-2", BaseURL: "https://api.deepseek.com"},
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

	cfg := validConfig()
	cfg.LLM = []LLMConfig{
		{Name: "chat", Provider: "deepseek", Model: "deepseek-chat", APIKey: "sk-1", BaseURL: "https://api.deepseek.com"},
		{Name: "CHAT", Provider: "deepseek", Model: "deepseek-reasoner", APIKey: "sk-2", BaseURL: "https://api.deepseek.com"},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected duplicate llm names to fail")
	}
}

func TestValidateRejectsInvalidRedisTTL(t *testing.T) {
	t.Parallel()

	cfg := validConfig()
	cfg.Redis.ShortTermTTL = "soon"
	cfg.Database.Redis = cfg.Redis

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected invalid redis ttl to fail")
	}
}

func TestLoadPanicsOnInvalidConfig(t *testing.T) {
	t.Parallel()

	path := writeConfig(t, "llm: []\n")

	defer expectPanic(t)
	_ = Load(path)
}

func validConfig() Config {
	return Config{
		LLM: []LLMConfig{{Provider: "mock", Model: "echo"}},
		Agent: AgentConfig{
			ID:                  "agent-1",
			Name:                "gora-eino-agent",
			Description:         "基于 Eino 的 Gora Agent",
			Instruction:         "你是一个智能助手，可以使用工具来完成任务。当需要获取外部信息时，请调用合适的工具。当你已经获得足够信息可以回答用户时，请直接给出回答。",
			MaxHistoryMessages:  30,
			MaxStreamChunkRunes: 64,
		},
	}
}

func withAgentConfig(content string) string {
	return strings.TrimSpace(content) + `

agent:
  id: agent-1
  name: gora-eino-agent
  description: 基于 Eino 的 Gora Agent
  instruction: 你是一个智能助手，可以使用工具来完成任务。当需要获取外部信息时，请调用合适的工具。当你已经获得足够信息可以回答用户时，请直接给出回答。
  max_history_messages: 30
  max_stream_chunk_runes: 64
`
}

func writeConfig(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func expectPanic(t *testing.T) {
	t.Helper()
	if recover() == nil {
		t.Fatal("expected panic")
	}
}
