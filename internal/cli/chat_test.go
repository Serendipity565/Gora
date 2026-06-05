package cli

import (
	"testing"
	"time"

	appconfig "github.com/Serendipity565/gora/internal/config"
	storage "github.com/Serendipity565/gora/internal/repository"
	"github.com/cloudwego/eino/schema"
)

func TestResolveStoredLLMIndexPrefersName(t *testing.T) {
	t.Parallel()

	cfg := testModelSelectionConfig()
	index, ok := resolveStoredLLMIndex(cfg, storage.ModelSelection{
		LLMName:  "reasoner",
		Model:    "deepseek-chat",
		LLMIndex: 0,
	})

	if !ok || index != 1 {
		t.Fatalf("expected reasoner index, got index=%d ok=%v", index, ok)
	}
}

func TestResolveStoredLLMIndexFallsBackToModel(t *testing.T) {
	t.Parallel()

	cfg := testModelSelectionConfig()
	index, ok := resolveStoredLLMIndex(cfg, storage.ModelSelection{
		Model: "deepseek-reasoner",
	})

	if !ok || index != 1 {
		t.Fatalf("expected model index, got index=%d ok=%v", index, ok)
	}
}

func TestResolveStoredLLMIndexRejectsMismatchedIndex(t *testing.T) {
	t.Parallel()

	cfg := testModelSelectionConfig()
	if index, ok := resolveStoredLLMIndex(cfg, storage.ModelSelection{
		Model:    "missing-model",
		LLMIndex: 1,
	}); ok {
		t.Fatalf("expected no match, got index=%d", index)
	}
}

func TestModelSelectionUserID(t *testing.T) {
	t.Setenv("GORA_USER_ID", "env-user")

	if got := modelSelectionUserID(cliOptions{}); got != "env-user" {
		t.Fatalf("expected env user id, got %q", got)
	}
	if got := modelSelectionUserID(cliOptions{UserID: "flag-user"}); got != "flag-user" {
		t.Fatalf("expected flag user id, got %q", got)
	}
}

func TestChatMessagesToHistories(t *testing.T) {
	t.Parallel()

	histories := chatMessagesToHistories("default", []storage.ChatMessage{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi"},
		{Role: "tool", Content: "ignored"},
	})
	history := histories["default"]
	if len(history) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(history))
	}
	if history[0].Role != schema.User || history[0].Content != "hello" {
		t.Fatalf("unexpected first message: %#v", history[0])
	}
	if history[1].Role != schema.Assistant || history[1].Content != "hi" {
		t.Fatalf("unexpected second message: %#v", history[1])
	}
}

func TestChatMessagesToHistoriesUsesProvidedSessionID(t *testing.T) {
	t.Parallel()

	histories := chatMessagesToHistories("session-2", []storage.ChatMessage{{
		Role:    "user",
		Content: "hello",
	}})

	if _, ok := histories["default"]; ok {
		t.Fatal("expected default session to be absent")
	}
	if got := len(histories["session-2"]); got != 1 {
		t.Fatalf("expected 1 message for session-2, got %d", got)
	}
}

func TestSchemaMessagesToChatMessages(t *testing.T) {
	t.Parallel()

	messages := schemaMessagesToChatMessages("user-1", "agent-1", "default", appconfig.LLMConfig{
		Name:  "chat",
		Model: "deepseek-chat",
	}, []*schema.Message{
		schema.UserMessage("hello"),
		schema.AssistantMessage("hi", nil),
		{Role: schema.Tool, Content: "ignored"},
	})

	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}
	if messages[0].Role != "user" || messages[0].Content != "hello" {
		t.Fatalf("unexpected first message: %#v", messages[0])
	}
	if messages[1].Role != "assistant" || messages[1].Model != "deepseek-chat" {
		t.Fatalf("unexpected second message: %#v", messages[1])
	}
}

func TestShortTermMemoryTTL(t *testing.T) {
	t.Parallel()

	got, err := shortTermMemoryTTL("")
	if err != nil {
		t.Fatalf("shortTermMemoryTTL failed: %v", err)
	}
	if got != 24*time.Hour {
		t.Fatalf("unexpected default ttl: %s", got)
	}
	got, err = shortTermMemoryTTL("30m")
	if err != nil {
		t.Fatalf("shortTermMemoryTTL failed: %v", err)
	}
	if got != 30*time.Minute {
		t.Fatalf("unexpected ttl: %s", got)
	}
	if _, err := shortTermMemoryTTL("soon"); err == nil {
		t.Fatal("expected invalid ttl to fail")
	}
}

func TestNormalizeSessionID(t *testing.T) {
	t.Parallel()

	if got := normalizeSessionID("  "); got != "default" {
		t.Fatalf("expected default session id, got %q", got)
	}
	if got := normalizeSessionID(" session-a "); got != "session-a" {
		t.Fatalf("expected trimmed session id, got %q", got)
	}
}

func TestApplyRuntimeOverridesUsesProviderEnvAPIKeys(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "sk-deepseek")
	t.Setenv("OPENAI_API_KEY", "sk-openai")

	cfg := appconfig.Config{
		LLM: []appconfig.LLMConfig{
			{Provider: "deepseek", Model: "deepseek-chat"},
			{Provider: "openai", Model: "gpt-4o-mini"},
		},
	}

	applyRuntimeOverrides(&cfg, cliOptions{RedisDB: -1})

	if cfg.LLM[0].APIKey != "sk-deepseek" {
		t.Fatalf("unexpected deepseek api key: %q", cfg.LLM[0].APIKey)
	}
	if cfg.LLM[1].APIKey != "sk-openai" {
		t.Fatalf("unexpected openai api key: %q", cfg.LLM[1].APIKey)
	}
}

func TestApplyRuntimeOverridesUsesNestedRedisConfig(t *testing.T) {
	t.Setenv("GORA_REDIS_ADDR", " 127.0.0.1:6379 ")
	t.Setenv("GORA_REDIS_PASSWORD", " secret ")
	t.Setenv("GORA_REDIS_DB", "2")
	t.Setenv("GORA_SHORT_TERM_MEMORY_TTL", " 12h ")

	cfg := appconfig.Config{}
	applyRuntimeOverrides(&cfg, cliOptions{RedisDB: -1})

	redis := cfg.RedisSettings()
	if redis.Addr != "127.0.0.1:6379" {
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

func testModelSelectionConfig() appconfig.Config {
	return appconfig.Config{
		LLM: []appconfig.LLMConfig{
			{Name: "chat", Model: "deepseek-chat"},
			{Name: "reasoner", Model: "deepseek-reasoner"},
		},
	}
}
