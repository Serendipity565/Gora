package configs

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
    api_style: openai
    api_key: "  sk-test  "
    base_url: "  https://api.deepseek.com  "
    model: "  deepseek-reasoner  "
database:
  mysql:
    addr: "  localhost:3306  "
    dbname: "  gora  "
    username: "  gora  "
    password: "  gora_dev_password  "
`))

	cfg := Load(path)

	if len(cfg.LLM) != 1 {
		t.Fatalf("expected 1 llm config, got %d", len(cfg.LLM))
	}
	if cfg.LLM[0].Name != "reasoner" {
		t.Fatalf("unexpected name: %q", cfg.LLM[0].Name)
	}
	if cfg.LLM[0].APIStyle != "openai" {
		t.Fatalf("unexpected api_style: %q", cfg.LLM[0].APIStyle)
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
	if cfg.Database.MySQL.Addr != "localhost:3306" {
		t.Fatalf("unexpected mysql addr: %q", cfg.Database.MySQL.Addr)
	}
}

func TestLoadSupportsOpenAIAPIStyle(t *testing.T) {
	t.Parallel()

	path := writeConfig(t, withAgentConfig(`
llm:
  - name: gpt-mini
    api_style: openai
    api_key: sk-openai
    base_url: https://api.openai.com/v1
    model: gpt-4o-mini
`))

	cfg := Load(path)

	if cfg.LLM[0].APIStyle != "openai" {
		t.Fatalf("unexpected api_style: %q", cfg.LLM[0].APIStyle)
	}
	if cfg.LLM[0].BaseURL != "https://api.openai.com/v1" {
		t.Fatalf("unexpected base url: %q", cfg.LLM[0].BaseURL)
	}
	if cfg.LLM[0].Model != "gpt-4o-mini" {
		t.Fatalf("unexpected model: %q", cfg.LLM[0].Model)
	}
}

func TestReadRedisConfig(t *testing.T) {
	t.Parallel()

	path := writeConfig(t, withAgentConfig(`
llm:
  - name: reasoner
    api_style: openai
    api_key: sk-test
    base_url: https://api.deepseek.com
    model: deepseek-chat
database:
  redis:
    addr: "  localhost:6379  "
    password: "  secret  "
    db: 2
`))

	cfg, err := Read(path)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	redis := cfg.Database.Redis
	if redis.Addr != "localhost:6379" {
		t.Fatalf("unexpected redis addr: %q", redis.Addr)
	}
	if redis.Password != "secret" {
		t.Fatalf("unexpected redis password: %q", redis.Password)
	}
	if redis.DB != 2 {
		t.Fatalf("unexpected redis db: %d", redis.DB)
	}
}

func TestFindLLM(t *testing.T) {
	t.Parallel()

	cfg := Config{
		LLM: []LLMConfig{
			{Name: "chat", APIStyle: "openai", Model: "deepseek-chat", APIKey: "sk-1", BaseURL: "https://api.deepseek.com"},
			{Name: "reasoner", APIStyle: "openai", Model: "deepseek-reasoner", APIKey: "sk-2", BaseURL: "https://api.deepseek.com"},
		},
	}

	// 命中：按 name（大小写不敏感）。
	if _, llm, err := cfg.FindLLM("chat"); err != nil || llm.Model != "deepseek-chat" {
		t.Fatalf("find by name failed: %v, %#v", err, llm)
	}
	if _, llm, err := cfg.FindLLM("REASONER"); err != nil || llm.Name != "reasoner" {
		t.Fatalf("find by name (case-insensitive) failed: %v, %#v", err, llm)
	}

	// 不再支持按 model 字符串查找。
	if _, _, err := cfg.FindLLM("deepseek-reasoner"); err == nil {
		t.Fatalf("expected lookup by model string to fail, but it succeeded")
	}

	// 不再支持按序号查找。
	if _, _, err := cfg.FindLLM("2"); err == nil {
		t.Fatalf("expected numeric selector to fail, but it succeeded")
	}

	// 空 selector 直接报错。
	if _, _, err := cfg.FindLLM("  "); err == nil {
		t.Fatalf("expected empty selector to fail")
	}
}

func TestValidateRequiresLLMName(t *testing.T) {
	t.Parallel()

	cfg := validConfig()
	cfg.LLM[0].Name = ""

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected missing llm name to fail validation")
	}
}

func TestValidateRejectsDuplicateLLMNames(t *testing.T) {
	t.Parallel()

	cfg := validConfig()
	cfg.LLM = []LLMConfig{
		{Name: "chat", APIStyle: "openai", Model: "deepseek-chat", APIKey: "sk-1", BaseURL: "https://api.deepseek.com"},
		{Name: "CHAT", APIStyle: "openai", Model: "deepseek-reasoner", APIKey: "sk-2", BaseURL: "https://api.deepseek.com"},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected duplicate llm names to fail")
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
		LLM: []LLMConfig{{Name: "chat", APIStyle: "openai", Model: "deepseek-chat", APIKey: "sk-test", BaseURL: "https://api.deepseek.com"}},
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
