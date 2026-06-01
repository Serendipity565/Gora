package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/Serendipity565/gora/llm"
	"github.com/Serendipity565/gora/tool"
)

// ReActConfig configures a ReAct Agent.
type ReActConfig struct {
	LLM          llm.LLM
	Tools        *tool.Registry
	SystemPrompt string
	MaxSteps     int
}

func DefaultReActConfig(llmClient llm.LLM, tools *tool.Registry) ReActConfig {
	return ReActConfig{
		LLM:   llmClient,
		Tools: tools,
		SystemPrompt: `你是一个智能助手，可以使用工具来完成任务。
当需要获取外部信息时，请调用合适的工具。
当你已经获得足够信息可以回答用户时，请直接给出回答，不要再调用工具。`,
		MaxSteps: 5,
	}
}

// ReActAgent implements the reason-act-observe loop.
type ReActAgent struct {
	*BaseAgent
	config ReActConfig

	muMessages sync.RWMutex
	messages   []llm.Message
}

func NewReActAgent(id string, config ReActConfig) *ReActAgent {
	if config.Tools == nil {
		config.Tools = tool.NewRegistry()
	}
	if config.MaxSteps <= 0 {
		config.MaxSteps = 5
	}

	return &ReActAgent{
		BaseAgent: NewBaseAgent(id, config.Tools),
		config:    config,
	}
}

func (a *ReActAgent) Run(ctx context.Context, input string) <-chan Event {
	events := make(chan Event, 32)
	stopCh, doneCh := a.beginRun()

	go func() {
		defer close(events)
		defer close(doneCh)

		send := func(event Event) bool {
			select {
			case events <- event:
				return true
			case <-ctx.Done():
				a.setState(StateError)
				return false
			case <-stopCh:
				return false
			}
		}

		if a.config.LLM == nil {
			_ = send(NewErrorEvent(a.id, fmt.Errorf("LLM 未配置")))
			a.setState(StateError)
			return
		}

		messages := a.prepareMessages(input)

		for step := 0; step < a.config.MaxSteps; step++ {
			select {
			case <-ctx.Done():
				_ = send(NewErrorEvent(a.id, ctx.Err()))
				a.setState(StateError)
				return
			case <-stopCh:
				return
			default:
			}

			if !send(NewThinkingEvent(a.id, fmt.Sprintf("步骤 %d: 正在思考...", step+1))) {
				return
			}

			toolSchemas := a.config.Tools.ToOpenAISchema()
			resp, err := a.config.LLM.Chat(ctx, messages, toolSchemas)
			if err != nil {
				_ = send(NewErrorEvent(a.id, fmt.Errorf("LLM 调用失败: %w", err)))
				a.setState(StateError)
				return
			}
			if resp == nil {
				_ = send(NewErrorEvent(a.id, fmt.Errorf("LLM 返回空消息")))
				a.setState(StateError)
				return
			}

			if len(resp.ToolCalls) > 0 {
				messages = append(messages, *resp)

				for _, tc := range resp.ToolCalls {
					toolName := tc.Function.Name
					args := parseToolArguments(tc.Function.Arguments)

					if !send(NewToolCallEvent(a.id, toolName, args)) {
						return
					}

					a.setState(StateWaiting)
					toolResult := a.executeTool(ctx, toolName, args)
					a.setState(StateRunning)

					if !send(NewToolResultEvent(a.id, toolName, toolResult)) {
						return
					}

					messages = append(messages, llm.Message{
						Role:       llm.RoleTool,
						Content:    toolResult,
						ToolCallID: tc.ID,
					})
				}

				continue
			}

			if resp.Content != "" {
				for _, chunk := range responseChunks(resp.Content) {
					if !send(NewChunkEvent(a.id, chunk)) {
						return
					}
				}
			}

			messages = append(messages, *resp)
			a.saveMessages(messages)

			if !send(NewDoneEvent(a.id)) {
				return
			}
			a.setState(StateDone)
			return
		}

		_ = send(NewErrorEvent(a.id, fmt.Errorf("达到最大思考步数 (%d)，请简化问题", a.config.MaxSteps)))
		a.setState(StateError)
	}()

	return events
}

func (a *ReActAgent) prepareMessages(input string) []llm.Message {
	a.muMessages.RLock()
	messages := make([]llm.Message, 0, len(a.messages)+2)
	messages = append(messages, a.messages...)
	a.muMessages.RUnlock()

	if len(messages) == 0 {
		messages = append(messages, llm.Message{Role: llm.RoleSystem, Content: a.config.SystemPrompt})
	}

	messages = append(messages, llm.Message{Role: llm.RoleUser, Content: input})
	return messages
}

func (a *ReActAgent) saveMessages(messages []llm.Message) {
	a.muMessages.Lock()
	defer a.muMessages.Unlock()

	a.messages = append(a.messages[:0], messages...)
}

func (a *ReActAgent) executeTool(ctx context.Context, toolName string, args map[string]any) string {
	t, ok := a.config.Tools.Get(toolName)
	if !ok {
		return fmt.Sprintf("错误: 工具 %s 不存在", toolName)
	}

	result, err := t.Execute(ctx, args)
	if err != nil {
		return fmt.Sprintf("错误: %s", err.Error())
	}

	return result
}

func parseToolArguments(raw string) map[string]any {
	args := make(map[string]any)
	if strings.TrimSpace(raw) == "" {
		return args
	}

	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return map[string]any{"raw": raw}
	}

	return args
}

func responseChunks(content string) []string {
	if content == "" {
		return nil
	}

	const maxRunes = 12
	if utf8.RuneCountInString(content) <= maxRunes {
		return []string{content}
	}

	chunks := make([]string, 0, utf8.RuneCountInString(content)/maxRunes+1)
	var builder strings.Builder
	count := 0

	for _, r := range content {
		builder.WriteRune(r)
		count++
		if count >= maxRunes {
			chunks = append(chunks, builder.String())
			builder.Reset()
			count = 0
		}
	}

	if builder.Len() > 0 {
		chunks = append(chunks, builder.String())
	}

	return chunks
}
