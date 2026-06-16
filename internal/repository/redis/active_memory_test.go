package redis

import (
	"context"
	"testing"
	"time"

	"github.com/Serendipity565/gora/internal/repository/model"
)

func TestOpenActiveMemoryCacheWithoutAddrUsesNoop(t *testing.T) {
	t.Parallel()

	store, err := OpenActiveMemoryCache(context.Background(), " ", "", 0, time.Hour)
	if err != nil {
		t.Fatalf("OpenActiveMemoryCache failed: %v", err)
	}
	if _, ok := store.(NoopActiveMemoryCache); !ok {
		t.Fatalf("expected noop cache, got %T", store)
	}
	if err := store.Save(context.Background(), 1, "agent-1", "default", []model.Message{{
		Role:    "user",
		Content: "hello",
	}}); err != nil {
		t.Fatalf("noop save failed: %v", err)
	}
	if _, ok, err := store.Get(context.Background(), 1, "agent-1", "default"); err != nil || ok {
		t.Fatalf("noop get failed: ok=%v err=%v", ok, err)
	}
}

func TestActiveMemoryKey(t *testing.T) {
	t.Parallel()

	key, err := activeMemoryKey(1, " agent-1 ", " default ")
	if err != nil {
		t.Fatalf("activeMemoryKey failed: %v", err)
	}
	if key != "gora:active-memory:1:agent-1:default" {
		t.Fatalf("unexpected key: %s", key)
	}
}
