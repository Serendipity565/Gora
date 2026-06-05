package llm

import (
	"context"
	"testing"
)

func TestNewEinoModelCreatesOpenAICompatibleModel(t *testing.T) {
	t.Parallel()

	model, err := NewEinoModel(context.Background(), ChatModelConfig{
		Provider: "deepseek",
		APIKey:   "sk-test",
		BaseURL:  "https://api.deepseek.com",
		Model:    "deepseek-chat",
	})
	if err != nil {
		t.Fatalf("NewEinoModel failed: %v", err)
	}
	if model == nil {
		t.Fatal("expected model")
	}
}

func TestNewEinoModelSupportsCustomProvider(t *testing.T) {
	t.Parallel()

	model, err := NewEinoModel(context.Background(), ChatModelConfig{
		Provider: "other",
		APIKey:   "sk-test",
		BaseURL:  "https://example.com/v1",
		Model:    "test",
	})
	if err != nil {
		t.Fatalf("NewEinoModel failed: %v", err)
	}
	if model == nil {
		t.Fatal("expected model")
	}
}
