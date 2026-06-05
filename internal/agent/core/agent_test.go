package core

import (
	"context"
	"testing"
	"time"

	"github.com/Serendipity565/gora/internal/agent/tool"
)

func TestBaseAgent_Run(t *testing.T) {
	registry := tool.NewRegistry()
	a := NewBaseAgent("test-agent", registry)

	if a.ID() != "test-agent" {
		t.Errorf("expected ID test-agent, got %s", a.ID())
	}

	if a.State() != StateIdle {
		t.Errorf("expected initial state idle, got %s", a.State())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	events := a.Run(ctx, "hello")

	var gotDone bool
	for event := range events {
		switch event.Type {
		case EventThinking:
			if event.AgentID != "test-agent" {
				t.Errorf("event AgentID mismatch")
			}
		case EventChunk:
		case EventDone:
			gotDone = true
		}
	}

	if !gotDone {
		t.Error("did not receive Done event")
	}

	if a.State() != StateDone {
		t.Errorf("expected final state done, got %s", a.State())
	}
}

func TestBaseAgent_RunTwice(t *testing.T) {
	registry := tool.NewRegistry()
	a := NewBaseAgent("test-agent", registry)

	for i := 0; i < 2; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		events := a.Run(ctx, "hello")
		for range events {
		}
		cancel()

		if a.State() != StateDone {
			t.Fatalf("run %d: expected final state done, got %s", i+1, a.State())
		}
	}
}

func TestBaseAgent_StopBeforeRun(t *testing.T) {
	registry := tool.NewRegistry()
	a := NewBaseAgent("test-agent", registry)

	done := make(chan error, 1)
	go func() {
		done <- a.Stop()
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Stop returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Stop blocked before Run")
	}
}

func TestBaseAgent_RejectsConcurrentRun(t *testing.T) {
	registry := tool.NewRegistry()
	a := NewBaseAgent("test-agent", registry)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	first := a.Run(ctx, "first")
	if event := <-first; event.Type != EventThinking {
		t.Fatalf("expected first run to start, got %s", event.Type)
	}

	second := a.Run(ctx, "second")
	event, ok := <-second
	if !ok {
		t.Fatal("second run closed without an error event")
	}
	if event.Type != EventError {
		t.Fatalf("expected concurrent run to be rejected, got %s", event.Type)
	}

	if err := a.Stop(); err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}
	for range first {
	}
}
