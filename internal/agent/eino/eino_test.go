package eino

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	einoadk "github.com/cloudwego/eino/adk"
	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/Serendipity565/gora/internal/agent/core"
	gotool "github.com/Serendipity565/gora/internal/agent/tool"
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
		case core.EventChunk:
			content += event.Content
		case core.EventDone:
			gotDone = true
		case core.EventError:
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
		case core.EventToolCall:
			gotToolCall = true
		case core.EventToolResult:
			gotToolResult = true
		case core.EventChunk:
			gotContent = true
		case core.EventDone:
			gotDone = true
		case core.EventError:
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

func TestEinoAgent_DisabledToolBlocksExecution(t *testing.T) {
	registry := gotool.NewRegistry()
	if err := registry.Register(&fakeSearchTool{}); err != nil {
		t.Fatalf("register tool failed: %v", err)
	}

	// 模型先尝试调用 mock_search，被用户拒绝后改为直接回答。
	model := &fakeToolCallingModel{
		responses: []*schema.Message{
			schema.AssistantMessage("", []schema.ToolCall{{
				ID:   "call_disabled",
				Type: "function",
				Function: schema.FunctionCall{
					Name:      "mock_search",
					Arguments: `{"query":"北京天气"}`,
				},
			}}),
			schema.AssistantMessage("抱歉，由于工具被关闭无法查询，请稍后再试。", nil),
		},
	}

	einoAgent, err := NewEinoAgent("test-eino-disabled", DefaultEinoConfig(model, registry))
	if err != nil {
		t.Fatalf("NewEinoAgent failed: %v", err)
	}

	gate := core.NewToolPermissionGate()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ctx = core.WithDisabledTools(ctx, []string{"mock_search"})
	ctx = core.WithToolPermissionGate(ctx, gate)

	events := einoAgent.Run(ctx, "北京天气怎么样？")

	var (
		permissionRequestID string
		gotPermissionEvent  bool
		gotToolCall         bool
		assistantContent    string
		gotDone             bool
	)
	for event := range events {
		switch event.Type {
		case core.EventToolPermissionRequest:
			gotPermissionEvent = true
			if id, ok := event.Metadata["request_id"].(string); ok {
				permissionRequestID = id
			}
			// 模拟用户点击「拒绝」。
			if !gate.Resolve(permissionRequestID, core.ToolPermissionDecision{Approved: false}) {
				t.Fatalf("expected gate.Resolve to hit pending entry %q", permissionRequestID)
			}
		case core.EventToolCall:
			gotToolCall = true
		case core.EventChunk:
			assistantContent += event.Content
		case core.EventDone:
			gotDone = true
		case core.EventError:
			t.Fatalf("unexpected error: %s", event.Content)
		}
	}

	if !gotPermissionEvent {
		t.Fatal("expected tool_permission_request event")
	}
	if permissionRequestID == "" {
		t.Fatal("expected request_id in permission event metadata")
	}
	if gotToolCall {
		t.Fatal("denied tool should not emit tool_call event")
	}
	if !gotDone {
		t.Fatal("expected done event after denial")
	}
	if !strings.Contains(assistantContent, "抱歉") {
		t.Fatalf("expected assistant to fall back gracefully, got %q", assistantContent)
	}
}

func TestEinoAgent_DisabledToolApprovedRunsTool(t *testing.T) {
	registry := gotool.NewRegistry()
	if err := registry.Register(&fakeSearchTool{}); err != nil {
		t.Fatalf("register tool failed: %v", err)
	}

	model := &fakeToolCallingModel{
		responses: []*schema.Message{
			schema.AssistantMessage("", []schema.ToolCall{{
				ID:   "call_approved",
				Type: "function",
				Function: schema.FunctionCall{
					Name:      "mock_search",
					Arguments: `{"query":"北京天气"}`,
				},
			}}),
			schema.AssistantMessage("根据搜索结果，今天北京晴朗 25°C。", nil),
		},
	}

	einoAgent, err := NewEinoAgent("test-eino-approve", DefaultEinoConfig(model, registry))
	if err != nil {
		t.Fatalf("NewEinoAgent failed: %v", err)
	}

	gate := core.NewToolPermissionGate()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ctx = core.WithDisabledTools(ctx, []string{"mock_search"})
	ctx = core.WithToolPermissionGate(ctx, gate)

	events := einoAgent.Run(ctx, "北京天气怎么样？")

	var (
		toolResult         string
		gotPermissionEvent bool
	)
	for event := range events {
		switch event.Type {
		case core.EventToolPermissionRequest:
			gotPermissionEvent = true
			id, _ := event.Metadata["request_id"].(string)
			// 模拟用户点击「允许」。
			if !gate.Resolve(id, core.ToolPermissionDecision{Approved: true}) {
				t.Fatalf("expected gate.Resolve to hit pending entry %q", id)
			}
		case core.EventToolResult:
			toolResult = event.Content
		case core.EventError:
			t.Fatalf("unexpected error: %s", event.Content)
		}
	}

	if !gotPermissionEvent {
		t.Fatal("expected tool_permission_request event")
	}
	if !strings.Contains(toolResult, "搜索结果") {
		t.Fatalf("expected real tool execution result, got %q", toolResult)
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
