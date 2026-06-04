package storage

import (
	"context"
	"testing"
)

func TestOpenModelSelectionStoreWithoutDatabaseURLUsesNoop(t *testing.T) {
	t.Parallel()

	store, err := OpenModelSelectionStore(context.Background(), " ")
	if err != nil {
		t.Fatalf("OpenModelSelectionStore failed: %v", err)
	}
	if _, ok := store.(NoopModelSelectionStore); !ok {
		t.Fatalf("expected noop store, got %T", store)
	}

	if err := store.Save(context.Background(), ModelSelection{}); err != nil {
		t.Fatalf("noop save failed: %v", err)
	}
	if _, ok, err := store.Get(context.Background(), "", "", ""); err != nil || ok {
		t.Fatalf("noop get failed: ok=%v err=%v", ok, err)
	}
	if err := store.AppendMessages(context.Background(), []ChatMessage{{
		UserID:    "local",
		AgentID:   "agent-1",
		SessionID: "default",
		Role:      "user",
		Content:   "hello",
	}}); err != nil {
		t.Fatalf("noop append messages failed: %v", err)
	}
	messages, err := store.ListRecentMessages(context.Background(), "local", "agent-1", "default", 10)
	if err != nil {
		t.Fatalf("noop list messages failed: %v", err)
	}
	if len(messages) != 0 {
		t.Fatalf("expected no noop messages, got %d", len(messages))
	}
}
