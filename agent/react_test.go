package agent

import (
	"context"
	"testing"
	"time"

	"github.com/Serendipity565/gora/llm"
	"github.com/Serendipity565/gora/tool"
)

type mockLLM struct {
	responses []*llm.Message
	callCount int
}

func (m *mockLLM) Chat(ctx context.Context, messages []llm.Message, tools []map[string]any) (*llm.Message, error) {
	if m.callCount < len(m.responses) {
		resp := m.responses[m.callCount]
		m.callCount++
		return resp, nil
	}

	return &llm.Message{Role: llm.RoleAssistant, Content: "默认回复"}, nil
}

func (m *mockLLM) ChatStream(ctx context.Context, messages []llm.Message, tools []map[string]any) <-chan llm.StreamChunk {
	ch := make(chan llm.StreamChunk)
	go func() {
		defer close(ch)
		resp, err := m.Chat(ctx, messages, tools)
		if err != nil {
			ch <- llm.StreamChunk{Error: err}
			return
		}
		ch <- llm.StreamChunk{Content: resp.Content, Done: true}
	}()

	return ch
}

type mockTool struct{}

func (m *mockTool) Name() string        { return "mock_search" }
func (m *mockTool) Description() string { return "模拟搜索工具" }
func (m *mockTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}
func (m *mockTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	return "搜索结果: 今天北京天气晴朗，25°C", nil
}

func TestReActAgent_ToolCalling(t *testing.T) {
	registry := tool.NewRegistry()
	if err := registry.Register(&mockTool{}); err != nil {
		t.Fatalf("register mock tool: %v", err)
	}

	mock := &mockLLM{
		responses: []*llm.Message{
			{
				Role:      llm.RoleAssistant,
				ToolCalls: []llm.ToolCall{newToolCall("call_1", "mock_search", `{"query":"北京天气"}`)},
			},
			{
				Role:    llm.RoleAssistant,
				Content: "北京今天天气晴朗，气温25°C。",
			},
		},
	}

	config := ReActConfig{
		LLM:          mock,
		Tools:        registry,
		SystemPrompt: "你是一个助手",
		MaxSteps:     5,
	}

	reactAgent := NewReActAgent("test-react", config)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	events := reactAgent.Run(ctx, "北京天气怎么样？")

	var (
		gotToolCall   bool
		gotToolResult bool
		gotContent    bool
		gotDone       bool
	)

	for event := range events {
		switch event.Type {
		case EventToolCall:
			gotToolCall = true
			if event.Content != "mock_search" {
				t.Errorf("expected tool name mock_search, got %s", event.Content)
			}
		case EventToolResult:
			gotToolResult = true
		case EventChunk:
			gotContent = true
		case EventDone:
			gotDone = true
		case EventError:
			t.Errorf("unexpected error: %s", event.Content)
		}
	}

	if !gotToolCall {
		t.Error("expected tool call")
	}
	if !gotToolResult {
		t.Error("expected tool result")
	}
	if !gotContent {
		t.Error("expected response content")
	}
	if !gotDone {
		t.Error("expected done event")
	}
	if reactAgent.State() != StateDone {
		t.Errorf("expected state done, got %s", reactAgent.State())
	}
}

func TestReActAgent_DirectAnswer(t *testing.T) {
	registry := tool.NewRegistry()

	mock := &mockLLM{
		responses: []*llm.Message{
			{
				Role:    llm.RoleAssistant,
				Content: "你好！有什么可以帮助你的？",
			},
		},
	}

	config := ReActConfig{
		LLM:          mock,
		Tools:        registry,
		SystemPrompt: "你是一个助手",
		MaxSteps:     5,
	}

	reactAgent := NewReActAgent("test-direct", config)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	events := reactAgent.Run(ctx, "你好")

	var finalContent string
	for event := range events {
		if event.Type == EventChunk {
			finalContent += event.Content
		}
		if event.Type == EventError {
			t.Errorf("unexpected error: %s", event.Content)
		}
	}

	if finalContent == "" {
		t.Error("expected response content")
	}
	t.Logf("response: %s", finalContent)
}

func newToolCall(id, name, arguments string) llm.ToolCall {
	var tc llm.ToolCall
	tc.ID = id
	tc.Type = "function"
	tc.Function.Name = name
	tc.Function.Arguments = arguments
	return tc
}
