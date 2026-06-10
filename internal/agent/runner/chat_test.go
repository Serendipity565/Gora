package runner

import (
	"testing"

	"github.com/cloudwego/eino/schema"

	appconfig "github.com/Serendipity565/gora/internal/config"
	storage "github.com/Serendipity565/gora/internal/repository"
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

// resolveStoredLLMIndex 现在只接受 LLMName。Model 单独存在不再被回退使用，
// 因为项目约定 name 是模型的唯一标识。
func TestResolveStoredLLMIndexRequiresName(t *testing.T) {
	t.Parallel()

	cfg := testModelSelectionConfig()
	if index, ok := resolveStoredLLMIndex(cfg, storage.ModelSelection{
		Model: "deepseek-reasoner",
	}); ok {
		t.Fatalf("expected lookup without LLMName to fail, got index=%d", index)
	}
}

func TestResolveStoredLLMIndexRejectsUnknownName(t *testing.T) {
	t.Parallel()

	cfg := testModelSelectionConfig()
	if index, ok := resolveStoredLLMIndex(cfg, storage.ModelSelection{
		LLMName:  "vanished",
		Model:    "missing-model",
		LLMIndex: 1,
	}); ok {
		t.Fatalf("expected unknown name to fail, got index=%d", index)
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

func TestNormalizeSessionID(t *testing.T) {
	t.Parallel()

	if got := normalizeSessionID("  "); got != "default" {
		t.Fatalf("expected default session id, got %q", got)
	}
	if got := normalizeSessionID(" session-a "); got != "session-a" {
		t.Fatalf("expected trimmed session id, got %q", got)
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
