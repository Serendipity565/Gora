// Package eino 基于 CloudWeGo Eino ADK 实现 core.Agent 接口。
package eino

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"

	einoadk "github.com/cloudwego/eino/adk"
	einomodel "github.com/cloudwego/eino/components/model"
	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"

	"github.com/Serendipity565/gora/internal/agent/core"
	gotool "github.com/Serendipity565/gora/internal/agent/tool"
)

// DefaultSessionID 是默认会话标识。
const DefaultSessionID = "default"

// EinoConfig 是基于 Eino 的 Agent 配置。
type EinoConfig struct {
	Model               einomodel.ToolCallingChatModel // Eino ToolCallingChatModel
	Tools               *gotool.Registry               // Gora 工具注册表
	Instruction         string                         // 系统提示词
	Name                string                         // Eino Agent 名称
	Description         string                         // Eino Agent 描述
	MaxHistoryMessages  int                            // 每个会话最多保留的历史消息数
	MaxStreamChunkRunes int                            // 单个输出事件的最大字符数
}

// DefaultEinoConfig 返回默认的 Eino Agent 配置。
func DefaultEinoConfig(model einomodel.ToolCallingChatModel, tools *gotool.Registry) EinoConfig {
	return EinoConfig{
		Model:               model,
		Tools:               tools,
		Instruction:         "你是一个智能助手，可以使用工具来完成任务。当需要获取外部信息时，请调用合适的工具。当你已经获得足够信息可以回答用户时，请直接给出回答。",
		Name:                "gora-eino-agent",
		Description:         "基于 Eino 的 Gora Agent",
		MaxHistoryMessages:  30,
		MaxStreamChunkRunes: 64,
	}
}

// EinoAgent 使用 Eino ChatModelAgent 作为推理内核，同时保留 Gora 的 goroutine 生命周期和事件流模型。
type EinoAgent struct {
	*core.BaseAgent
	config EinoConfig
	runner *einoadk.Runner

	muHistories sync.RWMutex
	histories   map[string][]*schema.Message
}

// SnapshotHistories 返回当前会话历史的深拷贝，供运行时重建 Agent 时复用上下文。
func (a *EinoAgent) SnapshotHistories() map[string][]*schema.Message {
	a.muHistories.RLock()
	defer a.muHistories.RUnlock()

	snapshot := make(map[string][]*schema.Message, len(a.histories))
	for sessionID, history := range a.histories {
		snapshot[sessionID] = append([]*schema.Message(nil), history...)
	}
	return snapshot
}

// RestoreHistories 用外部提供的会话历史替换当前 Agent 上下文。
func (a *EinoAgent) RestoreHistories(histories map[string][]*schema.Message) {
	a.muHistories.Lock()
	defer a.muHistories.Unlock()

	a.histories = make(map[string][]*schema.Message, len(histories))
	for sessionID, history := range histories {
		a.histories[sessionID] = append([]*schema.Message(nil), history...)
	}
}

// NewEinoAgent 创建一个基于 Eino 的 Agent。
func NewEinoAgent(id string, config EinoConfig) (*EinoAgent, error) {
	if config.Model == nil {
		return nil, fmt.Errorf("Eino Model 未配置")
	}
	if config.Tools == nil {
		config.Tools = gotool.NewRegistry()
	}
	if strings.TrimSpace(config.Name) == "" {
		config.Name = id
	}
	if strings.TrimSpace(config.Description) == "" {
		config.Description = "Gora Eino Agent"
	}
	if config.MaxHistoryMessages <= 0 {
		config.MaxHistoryMessages = 30
	}
	if config.MaxStreamChunkRunes <= 0 {
		config.MaxStreamChunkRunes = 64
	}

	tools, err := buildEinoTools(config.Tools)
	if err != nil {
		return nil, err
	}

	einoAgent, err := einoadk.NewChatModelAgent(context.Background(), &einoadk.ChatModelAgentConfig{
		Name:        config.Name,
		Description: config.Description,
		Instruction: config.Instruction,
		Model:       config.Model,
		ToolsConfig: einoadk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: tools,
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("创建 Eino Agent 失败: %w", err)
	}

	return &EinoAgent{
		BaseAgent: core.NewBaseAgent(id, config.Tools),
		config:    config,
		runner: einoadk.NewRunner(context.Background(), einoadk.RunnerConfig{
			Agent:           einoAgent,
			EnableStreaming: true,
		}),
		histories: make(map[string][]*schema.Message),
	}, nil
}

// Run 使用默认会话执行一轮对话。
func (a *EinoAgent) Run(ctx context.Context, input string) <-chan core.Event {
	return a.RunSession(ctx, DefaultSessionID, input)
}

// RunSession 在指定会话中执行一轮对话。
func (a *EinoAgent) RunSession(ctx context.Context, sessionID, input string) <-chan core.Event {
	events := make(chan core.Event, 32)
	runCtx, stopCh, doneCh, ok := a.BeginRun(ctx)
	if !ok {
		go func() {
			defer close(events)
			events <- core.NewErrorEvent(a.ID(), fmt.Errorf("agent %s is already running", a.ID()))
		}()
		return events
	}

	go func() {
		defer close(events)
		defer a.FinishRun(doneCh)

		send := func(event core.Event) bool {
			select {
			case events <- event:
				return true
			case <-runCtx.Done():
				select {
				case <-stopCh:
					return false
				default:
				}
				a.SetState(core.StateError)
				return false
			case <-stopCh:
				return false
			}
		}

		// toolEventBuf 缓冲 InvokableRun 发出的 tool_call/tool_result 事件，
		// 由 flushToolEvents 在主循环中按序排空，确保工具事件不会与流式 chunk 交错。
		var toolEventBuf []core.Event
		var toolEventMu sync.Mutex

		toolSend := func(event core.Event) bool {
			toolEventMu.Lock()
			toolEventBuf = append(toolEventBuf, event)
			toolEventMu.Unlock()
			return true
		}

		flushToolEvents := func() bool {
			toolEventMu.Lock()
			buf := toolEventBuf
			toolEventBuf = nil
			toolEventMu.Unlock()
			for _, e := range buf {
				if !send(e) {
					return false
				}
			}
			return true
		}

		if strings.TrimSpace(sessionID) == "" {
			sessionID = DefaultSessionID
		}

		messages := a.prepareMessages(sessionID, input)
		runCtx = withToolEventEmitter(runCtx, toolSend, a.ID())

		if !send(core.NewThinkingEvent(a.ID(), "正在使用 Eino Agent 思考...")) {
			return
		}

		iter := a.runner.Run(runCtx, messages)
		var assistantContents []string

		for {
			event, ok := iter.Next()
			if !ok {
				break
			}
			if event == nil {
				continue
			}
			if event.Err != nil {
				if !send(core.NewErrorEvent(a.ID(), fmt.Errorf("Eino 执行失败: %w", event.Err))) {
					return
				}
				a.SetState(core.StateError)
				return
			}
			if event.Output == nil || event.Output.MessageOutput == nil {
				continue
			}

			// 在处理新一轮模型输出之前，先排空上一轮工具调用产生的事件，
			// 保证 tool_call/tool_result 按序出现在模型回答之前。
			if !flushToolEvents() {
				return
			}

			content, role, ok := a.forwardMessageOutput(event.Output.MessageOutput, send)
			if !ok {
				return
			}
			if role == schema.Assistant && strings.TrimSpace(content) != "" {
				assistantContents = append(assistantContents, content)
			}
		}

		// 排空可能残留的工具事件（流被客户端提前中断等场景）
		flushToolEvents()

		finalReply := strings.TrimSpace(strings.Join(assistantContents, "\n"))
		a.saveMessages(sessionID, input, finalReply)

		if !send(core.NewDoneEvent(a.ID())) {
			return
		}
		a.SetState(core.StateDone)
	}()

	return events
}

func (a *EinoAgent) forwardMessageOutput(
	output *einoadk.MessageVariant,
	send func(core.Event) bool,
) (string, schema.RoleType, bool) {
	if output == nil {
		return "", "", true
	}

	// 工具事件已经由工具适配器主动发出，这里只转发 assistant 输出，避免重复。
	if output.Role == schema.Tool {
		return messageContent(output), output.Role, true
	}

	content, err := readMessageVariant(output, a.config.MaxStreamChunkRunes, func(part string) bool {
		return send(core.NewChunkEvent(a.ID(), part))
	})
	if err != nil {
		_ = send(core.NewErrorEvent(a.ID(), fmt.Errorf("读取 Eino 输出失败: %w", err)))
		a.SetState(core.StateError)
		return "", output.Role, false
	}

	return content, output.Role, true
}

func (a *EinoAgent) prepareMessages(sessionID, input string) []*schema.Message {
	a.muHistories.RLock()
	history := append([]*schema.Message(nil), a.histories[sessionID]...)
	a.muHistories.RUnlock()

	history = append(history, schema.UserMessage(input))
	return history
}

func (a *EinoAgent) saveMessages(sessionID, input, reply string) {
	a.muHistories.Lock()
	defer a.muHistories.Unlock()

	history := append([]*schema.Message(nil), a.histories[sessionID]...)
	history = append(history, schema.UserMessage(input))
	if strings.TrimSpace(reply) != "" {
		history = append(history, schema.AssistantMessage(reply, nil))
	}
	a.histories[sessionID] = trimSchemaHistory(history, a.config.MaxHistoryMessages)
}

func buildEinoTools(registry *gotool.Registry) ([]einotool.BaseTool, error) {
	tools := registry.List()
	out := make([]einotool.BaseTool, 0, len(tools))
	for _, t := range tools {
		info, err := buildToolInfo(t)
		if err != nil {
			return nil, fmt.Errorf("构建工具 %s 的 Eino 描述失败: %w", t.Name(), err)
		}
		out = append(out, &goraInvokableTool{
			tool: t,
			info: info,
		})
	}
	return out, nil
}

func buildToolInfo(t gotool.Tool) (*schema.ToolInfo, error) {
	payload := map[string]any{
		"name": t.Name(),
		"desc": t.Description(),
	}
	if params := t.Parameters(); params != nil {
		payload["has_params_one_of"] = true
		payload["json_schema"] = params
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	var info schema.ToolInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, err
	}

	return &info, nil
}

func trimSchemaHistory(history []*schema.Message, max int) []*schema.Message {
	if max <= 0 || len(history) <= max {
		return history
	}
	return append([]*schema.Message(nil), history[len(history)-max:]...)
}

func readMessageVariant(
	output *einoadk.MessageVariant,
	maxChunkRunes int,
	onChunk func(string) bool,
) (string, error) {
	if output == nil {
		return "", nil
	}

	if !output.IsStreaming {
		content := messageContent(output)
		for _, part := range splitRunes(content, maxChunkRunes) {
			if part == "" {
				continue
			}
			if !onChunk(part) {
				return "", context.Canceled
			}
		}
		return content, nil
	}

	defer output.MessageStream.Close()

	var content strings.Builder
	for {
		frame, err := output.MessageStream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		if frame == nil || frame.Content == "" {
			continue
		}
		content.WriteString(frame.Content)
		for _, part := range splitRunes(frame.Content, maxChunkRunes) {
			if part == "" {
				continue
			}
			if !onChunk(part) {
				return "", context.Canceled
			}
		}
	}

	return content.String(), nil
}

func messageContent(output *einoadk.MessageVariant) string {
	if output == nil || output.Message == nil {
		return ""
	}
	return output.Message.Content
}

func splitRunes(content string, maxRunes int) []string {
	if maxRunes <= 0 || len([]rune(content)) <= maxRunes {
		return []string{content}
	}

	runes := []rune(content)
	parts := make([]string, 0, len(runes)/maxRunes+1)
	for start := 0; start < len(runes); start += maxRunes {
		end := start + maxRunes
		if end > len(runes) {
			end = len(runes)
		}
		parts = append(parts, string(runes[start:end]))
	}

	return parts
}

type toolEventEmitter func(core.Event) bool

type toolEventContextKey struct{}

type toolEventPayload struct {
	emit    toolEventEmitter
	agentID string
}

func withToolEventEmitter(ctx context.Context, emit toolEventEmitter, agentID string) context.Context {
	return context.WithValue(ctx, toolEventContextKey{}, toolEventPayload{
		emit:    emit,
		agentID: agentID,
	})
}

func getToolEventEmitter(ctx context.Context) (toolEventPayload, bool) {
	payload, ok := ctx.Value(toolEventContextKey{}).(toolEventPayload)
	return payload, ok
}

type goraInvokableTool struct {
	tool gotool.Tool
	info *schema.ToolInfo
}

func (g *goraInvokableTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return g.info, nil
}

func (g *goraInvokableTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...einotool.Option) (string, error) {
	args := make(map[string]any)
	if strings.TrimSpace(argumentsInJSON) != "" {
		if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
			args = map[string]any{"raw": argumentsInJSON}
		}
	}

	// 命中"被前端关闭"的工具：
	//   - 若 ctx 中有 ToolPermissionGate，则发起一次询问，等用户答复后再决定执行 / 拒绝；
	//   - 否则直接拒绝并把原因回写给模型。
	if core.IsToolDisabled(ctx, g.tool.Name()) {
		approved, err := g.askPermission(ctx, args)
		if err != nil {
			return "", err
		}
		if !approved {
			return refusalMessage(g.tool.Name()), nil
		}
		// 用户允许后继续走正常执行路径。
	}

	if payload, ok := getToolEventEmitter(ctx); ok {
		if !payload.emit(core.NewToolCallEvent(payload.agentID, g.tool.Name(), args)) {
			return "", context.Canceled
		}
	}

	result, err := g.tool.Execute(ctx, args)
	if err != nil {
		result = fmt.Sprintf("错误: %s", err.Error())
		err = nil
	}

	if payload, ok := getToolEventEmitter(ctx); ok {
		if !payload.emit(core.NewToolResultEvent(payload.agentID, g.tool.Name(), result)) {
			return "", context.Canceled
		}
	}

	return result, err
}

// askPermission 向前端发起"是否允许调用 toolName"的询问，等待用户答复。
// 返回值表示是否被允许执行；err 仅在 ctx 取消等异常时返回。
func (g *goraInvokableTool) askPermission(ctx context.Context, args map[string]any) (bool, error) {
	gate, ok := core.ToolPermissionGateFromContext(ctx)
	if !ok {
		// 没有 gate 时退化为"直接拒绝"。
		return false, nil
	}

	requestID := core.NewRequestID()
	waitCh, err := gate.Register(requestID)
	if err != nil {
		return false, nil
	}

	payload, hasEmitter := getToolEventEmitter(ctx)
	if hasEmitter {
		if !payload.emit(core.NewToolPermissionRequestEvent(payload.agentID, requestID, g.tool.Name(), args)) {
			gate.Cancel(requestID)
			return false, context.Canceled
		}
	}

	select {
	case <-ctx.Done():
		gate.Cancel(requestID)
		return false, ctx.Err()
	case decision, alive := <-waitCh:
		if !alive {
			// gate 被外部 Cancel 掉，按拒绝处理。
			return false, nil
		}
		return decision.Approved, nil
	}
}

// refusalMessage 构造拒绝提示，会被作为 tool 的返回值回写给模型，
// 同时也通过 tool_result 事件发送到前端。
func refusalMessage(toolName string) string {
	return fmt.Sprintf("用户拒绝授权调用工具 %s，请改用其他工具或直接回答。", toolName)
}
