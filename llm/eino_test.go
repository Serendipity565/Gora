package llm

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestNewEinoModelMockEchoesLastUserInput(t *testing.T) {
	t.Parallel()

	model, err := NewEinoModel(context.Background(), ChatModelConfig{
		Provider: "mock",
		Model:    "echo",
	})
	if err != nil {
		t.Fatalf("NewEinoModel failed: %v", err)
	}

	message, err := model.Generate(context.Background(), []*schema.Message{
		schema.UserMessage("第一句"),
		schema.AssistantMessage("中间回复", nil),
		schema.UserMessage("最后一句"),
	})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if message.Content != "最后一句" {
		t.Fatalf("unexpected content: %q", message.Content)
	}
}

func TestNewEinoModelRejectsUnsupportedProvider(t *testing.T) {
	t.Parallel()

	if _, err := NewEinoModel(context.Background(), ChatModelConfig{
		Provider: "other",
		APIKey:   "sk-test",
		BaseURL:  "https://example.com/v1",
		Model:    "test",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
