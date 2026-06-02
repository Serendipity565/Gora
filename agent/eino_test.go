package agent

import (
	"context"
	"io"
	"testing"
	"time"

	einoadk "github.com/cloudwego/eino/adk"
	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	gotool "github.com/Serendipity565/gora/tool"
)

type fakeToolCallingModel struct {
	responses []*schema.Message
	callCount int
	tools     []*schema.ToolInfo
}

func (f *fakeToolCallingModel) Generate(ctx context.Context, input []*schema.Message, opts ...einomodel.Option) (*schema.Message, error) {
	if f.callCount >= len(f.responses) {
		return schema.AssistantMessage("默认回复", nil), nil
	}
	resp := f.responses[f.callCount]
	f.callCount++
	return resp, nil
}

func (f *fakeToolCallingModel) Stream(ctx context.Context, input []*schema.Message, opts ...einomodel.Option) (*schema.StreamReader[*schema.Message], error) {
	msg, err := f.Generate(ctx, input, opts...)
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]*schema.Message{msg}), nil
}

func (f *fakeToolCallingModel) WithTools(tools []*schema.ToolInfo) (einomodel.ToolCallingChatModel, error) {
	clone := *f
	clone.tools = append([]*schema.ToolInfo(nil), tools...)
	return &clone, nil
}

type fakeSearchTool struct{}

func (f *fakeSearchTool) Name() string        { return "mock_search" }
func (f *fakeSearchTool) Description() string { return "模拟搜索工具" }
func (f *fakeSearchTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{"type": "string"},
		},
		"required": []string{"query"},
	}
}
func (f *fakeSearchTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	return "搜索结果: 今天北京天气晴朗，25°C", nil
}

func TestEinoAgent_DirectAnswer(t *testing.T) {
	model := &fakeToolCallingModel{
		responses: []*schema.Message{
			schema.AssistantMessage("你好！有什么可以帮助你的？", nil),
		},
	}

	agent, err := NewEinoAgent("test-eino-direct", DefaultEinoConfig(model, gotool.NewRegistry()))
	if err != nil {
		t.Fatalf("NewEinoAgent failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	events := agent.Run(ctx, "你好")

	var (
		gotDone bool
		content string
	)
	for event := range events {
		switch event.Type {
		case EventChunk:
			content += event.Content
		case EventDone:
			gotDone = true
		case EventError:
			t.Fatalf("unexpected error: %s", event.Content)
		}
	}

	if content == "" {
		t.Fatal("expected assistant content")
	}
	if !gotDone {
		t.Fatal("expected done event")
	}
}

func TestEinoAgent_ToolCalling(t *testing.T) {
	registry := gotool.NewRegistry()
	if err := registry.Register(&fakeSearchTool{}); err != nil {
		t.Fatalf("register tool failed: %v", err)
	}

	model := &fakeToolCallingModel{
		responses: []*schema.Message{
			schema.AssistantMessage("", []schema.ToolCall{{
				ID:   "call_1",
				Type: "function",
				Function: schema.FunctionCall{
					Name:      "mock_search",
					Arguments: `{"query":"北京天气"}`,
				},
			}}),
			schema.AssistantMessage("北京今天天气晴朗，气温25°C。", nil),
		},
	}

	agent, err := NewEinoAgent("test-eino-tool", DefaultEinoConfig(model, registry))
	if err != nil {
		t.Fatalf("NewEinoAgent failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	events := agent.Run(ctx, "北京天气怎么样？")

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
		case EventToolResult:
			gotToolResult = true
		case EventChunk:
			gotContent = true
		case EventDone:
			gotDone = true
		case EventError:
			t.Fatalf("unexpected error: %s", event.Content)
		}
	}

	if !gotToolCall {
		t.Fatal("expected tool call event")
	}
	if !gotToolResult {
		t.Fatal("expected tool result event")
	}
	if !gotContent {
		t.Fatal("expected content chunks")
	}
	if !gotDone {
		t.Fatal("expected done event")
	}
}

func TestReadMessageVariant_Stream(t *testing.T) {
	stream := schema.StreamReaderFromArray([]*schema.Message{
		schema.AssistantMessage("你好", nil),
		schema.AssistantMessage("，世界", nil),
	})

	var parts []string
	content, err := readMessageVariant(&einoadk.MessageVariant{
		IsStreaming:   true,
		MessageStream: stream,
		Role:          schema.Assistant,
	}, 2, func(part string) bool {
		parts = append(parts, part)
		return true
	})
	if err != nil && err != io.EOF {
		t.Fatalf("unexpected error: %v", err)
	}
	if content != "你好，世界" {
		t.Fatalf("unexpected content: %s", content)
	}
	if len(parts) == 0 {
		t.Fatal("expected chunk parts")
	}
}
