package agent

import (
	"context"
	"time"
)

// State represents the current state of an Agent.
type State int

const (
	StateIdle    State = iota // Idle
	StateRunning              // Running
	StateWaiting              // Waiting for tool results or external work
	StateDone                 // Done
	StateError                // Error
)

func (s State) String() string {
	switch s {
	case StateIdle:
		return "idle"
	case StateRunning:
		return "running"
	case StateWaiting:
		return "waiting"
	case StateDone:
		return "done"
	case StateError:
		return "error"
	default:
		return "unknown"
	}
}

// EventType defines runtime event types emitted by an Agent.
type EventType int

const (
	EventThinking   EventType = iota // Agent is thinking
	EventToolCall                    // Agent is calling a tool
	EventToolResult                  // Tool returned a result
	EventChunk                       // Streaming LLM output chunk
	EventDone                        // Current turn is done
	EventError                       // Error occurred
)

func (e EventType) String() string {
	switch e {
	case EventThinking:
		return "thinking"
	case EventToolCall:
		return "tool_call"
	case EventToolResult:
		return "tool_result"
	case EventChunk:
		return "chunk"
	case EventDone:
		return "done"
	case EventError:
		return "error"
	default:
		return "unknown"
	}
}

// Event is emitted while an Agent runs.
type Event struct {
	Type      EventType      `json:"type"`
	AgentID   string         `json:"agent_id"`
	Content   string         `json:"content"`
	Timestamp time.Time      `json:"timestamp"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// Agent is the core intelligent agent interface.
type Agent interface {
	ID() string
	Run(ctx context.Context, input string) <-chan Event
	State() State
	Stop() error
}
