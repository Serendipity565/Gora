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

const DefaultSessionID = "default"

// ReActConfig 是 ReAct Agent 的运行配置。
type ReActConfig struct {
	LLM                 llm.LLM        // 用于推理和决策的大模型客户端
	Tools               *tool.Registry // Agent 可调用的工具注册表
	SystemPrompt        string         // 系统提示词，定义 Agent 的行为边界
	MaxSteps            int            // 最大思考轮数，防止无限循环
	MaxHistoryMessages  int            // 每个会话最多保留的历史消息数（不含系统消息）
	MaxToolResultRunes  int            // 工具结果写入上下文前的最大字符数
	MaxStreamChunkRunes int            // 单个输出事件的最大字符数
}

// DefaultReActConfig 返回一份适合普通助手场景的默认 ReAct 配置。
func DefaultReActConfig(llmClient llm.LLM, tools *tool.Registry) ReActConfig {
	return ReActConfig{
		LLM:   llmClient,
		Tools: tools,
		SystemPrompt: `你是一个智能助手，可以使用工具来完成任务。
当需要获取外部信息时，请调用合适的工具。
当你已经获得足够信息可以回答用户时，请直接给出回答，不要再调用工具。`,
		MaxSteps:            5,
		MaxHistoryMessages:  30,
		MaxToolResultRunes:  4000,
		MaxStreamChunkRunes: 64,
	}
}

// ReActAgent 实现“思考-行动-观察”的 ReAct 循环。
type ReActAgent struct {
	*BaseAgent
	config ReActConfig

	muHistories sync.RWMutex
	histories   map[string][]llm.Message
}

// NewReActAgent 创建一个 ReAct Agent，并补齐必要的默认配置。
func NewReActAgent(id string, config ReActConfig) *ReActAgent {
	if config.Tools == nil {
		config.Tools = tool.NewRegistry()
	}
	if config.MaxSteps <= 0 {
		config.MaxSteps = 5
	}
	if config.MaxHistoryMessages <= 0 {
		config.MaxHistoryMessages = 30
	}
	if config.MaxToolResultRunes <= 0 {
		config.MaxToolResultRunes = 4000
	}
	if config.MaxStreamChunkRunes <= 0 {
		config.MaxStreamChunkRunes = 64
	}

	return &ReActAgent{
		BaseAgent: NewBaseAgent(id, config.Tools),
		config:    config,
		histories: make(map[string][]llm.Message),
	}
}

// Run 使用默认会话执行一轮 ReAct 循环。
func (a *ReActAgent) Run(ctx context.Context, input string) <-chan Event {
	return a.RunSession(ctx, DefaultSessionID, input)
}

// RunSession 在指定会话中执行一轮 ReAct 循环，并通过事件通道返回运行过程。
func (a *ReActAgent) RunSession(ctx context.Context, sessionID, input string) <-chan Event {
	events := make(chan Event, 32)
	runCtx, stopCh, doneCh, ok := a.beginRun(ctx)
	if !ok {
		go func() {
			defer close(events)
			events <- NewErrorEvent(a.id, fmt.Errorf("agent %s is already running", a.id))
		}()
		return events
	}

	go func() {
		defer close(events)
		defer a.finishRun(doneCh)

		send := func(event Event) bool {
			select {
			case events <- event:
				return true
			case <-runCtx.Done():
				select {
				case <-stopCh:
					return false
				default:
				}
				a.setState(StateError)
				return false
			case <-stopCh:
				return false
			}
		}

		if strings.TrimSpace(sessionID) == "" {
			sessionID = DefaultSessionID
		}

		if a.config.LLM == nil {
			_ = send(NewErrorEvent(a.id, fmt.Errorf("LLM 未配置")))
			a.setState(StateError)
			return
		}

		messages := a.prepareMessages(sessionID, input)

		for step := 0; step < a.config.MaxSteps; step++ {
			select {
			case <-runCtx.Done():
				select {
				case <-stopCh:
					return
				default:
				}
				_ = send(NewErrorEvent(a.id, runCtx.Err()))
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
			resp, ok := a.streamAssistant(runCtx, messages, toolSchemas, send, stopCh)
			if !ok {
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
					toolResult := a.executeTool(runCtx, toolName, args)
					a.setState(StateRunning)

					if !send(NewToolResultEvent(a.id, toolName, toolResult)) {
						return
					}

					messages = append(messages, llm.Message{
						Role:       llm.RoleTool,
						Content:    truncateRunes(toolResult, a.config.MaxToolResultRunes),
						ToolCallID: tc.ID,
					})
				}

				continue
			}

			messages = append(messages, *resp)
			a.saveMessages(sessionID, messages)

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

func (a *ReActAgent) streamAssistant(
	ctx context.Context,
	messages []llm.Message,
	toolSchemas []map[string]any,
	send func(Event) bool,
	stopCh <-chan struct{},
) (*llm.Message, bool) {
	var content strings.Builder
	var toolCalls []llm.ToolCall

	chunks := a.config.LLM.ChatStream(ctx, messages, toolSchemas)
	for {
		select {
		case chunk, ok := <-chunks:
			if !ok {
				return &llm.Message{
					Role:      llm.RoleAssistant,
					Content:   content.String(),
					ToolCalls: toolCalls,
				}, true
			}

			if chunk.Error != nil {
				select {
				case <-stopCh:
					return nil, false
				default:
				}
				_ = send(NewErrorEvent(a.id, fmt.Errorf("LLM 调用失败: %w", chunk.Error)))
				a.setState(StateError)
				return nil, false
			}

			if len(chunk.ToolCalls) > 0 {
				toolCalls = chunk.ToolCalls
			}

			if len(toolCalls) == 0 && chunk.Content != "" {
				content.WriteString(chunk.Content)
				for _, part := range splitRunes(chunk.Content, a.config.MaxStreamChunkRunes) {
					if !send(NewChunkEvent(a.id, part)) {
						return nil, false
					}
				}
			}

			if chunk.Done {
				return &llm.Message{
					Role:      llm.RoleAssistant,
					Content:   content.String(),
					ToolCalls: toolCalls,
				}, true
			}
		case <-ctx.Done():
			select {
			case <-stopCh:
				return nil, false
			default:
			}
			_ = send(NewErrorEvent(a.id, ctx.Err()))
			a.setState(StateError)
			return nil, false
		case <-stopCh:
			return nil, false
		}
	}
}

// prepareMessages 复制指定会话的历史消息，并追加本轮用户输入。
func (a *ReActAgent) prepareMessages(sessionID, input string) []llm.Message {
	a.muHistories.RLock()
	history := a.histories[sessionID]
	messages := make([]llm.Message, 0, len(history)+2)
	messages = append(messages, history...)
	a.muHistories.RUnlock()

	if len(messages) == 0 {
		messages = append(messages, llm.Message{Role: llm.RoleSystem, Content: a.config.SystemPrompt})
	}

	messages = append(messages, llm.Message{Role: llm.RoleUser, Content: input})
	return messages
}

// saveMessages 保存指定会话本轮完成后的对话历史。
func (a *ReActAgent) saveMessages(sessionID string, messages []llm.Message) {
	a.muHistories.Lock()
	defer a.muHistories.Unlock()

	a.histories[sessionID] = trimMessages(messages, a.config.MaxHistoryMessages)
}

// executeTool 根据工具名从注册表中查找并执行工具。
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

// parseToolArguments 把模型返回的 JSON 字符串参数转换为 map。
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

func trimMessages(messages []llm.Message, maxHistoryMessages int) []llm.Message {
	if maxHistoryMessages <= 0 || len(messages) <= maxHistoryMessages+1 {
		return append([]llm.Message(nil), messages...)
	}

	var system []llm.Message
	rest := messages
	if len(messages) > 0 && messages[0].Role == llm.RoleSystem {
		system = append(system, messages[0])
		rest = messages[1:]
	}

	if len(rest) > maxHistoryMessages {
		rest = rest[len(rest)-maxHistoryMessages:]
	}

	for len(rest) > 0 && (rest[0].Role == llm.RoleTool || len(rest[0].ToolCalls) > 0) {
		rest = rest[1:]
	}

	trimmed := make([]llm.Message, 0, len(system)+len(rest))
	trimmed = append(trimmed, system...)
	trimmed = append(trimmed, rest...)
	return trimmed
}

func splitRunes(s string, max int) []string {
	if s == "" {
		return nil
	}
	if max <= 0 || utf8.RuneCountInString(s) <= max {
		return []string{s}
	}

	parts := make([]string, 0, utf8.RuneCountInString(s)/max+1)
	var builder strings.Builder
	count := 0

	for _, r := range s {
		builder.WriteRune(r)
		count++
		if count >= max {
			parts = append(parts, builder.String())
			builder.Reset()
			count = 0
		}
	}

	if builder.Len() > 0 {
		parts = append(parts, builder.String())
	}

	return parts
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return s
	}

	runes := []rune(s)
	if len(runes) <= max {
		return s
	}

	return string(runes[:max]) + "...(truncated)"
}
