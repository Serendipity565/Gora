package storage

import (
	"context"
	"testing"
	"time"
)

func TestOpenActiveMemoryStoreWithoutAddrUsesNoop(t *testing.T) {
	t.Parallel()

	store, err := OpenActiveMemoryStore(context.Background(), " ", "", 0, time.Hour)
	if err != nil {
		t.Fatalf("OpenActiveMemoryStore failed: %v", err)
	}
	if _, ok := store.(NoopActiveMemoryStore); !ok {
		t.Fatalf("expected noop store, got %T", store)
	}
	if err := store.Save(context.Background(), "local", "agent-1", "default", []ChatMessage{{
		Role:    "user",
		Content: "hello",
	}}); err != nil {
		t.Fatalf("noop save failed: %v", err)
	}
	if _, ok, err := store.Get(context.Background(), "local", "agent-1", "default"); err != nil || ok {
		t.Fatalf("noop get failed: ok=%v err=%v", ok, err)
	}
}

func TestActiveMemoryKey(t *testing.T) {
	t.Parallel()

	key, err := activeMemoryKey("", " agent-1 ", " default ")
	if err != nil {
		t.Fatalf("activeMemoryKey failed: %v", err)
	}
	if key != "gora:active-memory:local:agent-1:default" {
		t.Fatalf("unexpected key: %s", key)
	}
}
