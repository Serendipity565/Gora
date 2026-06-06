package app

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"

	"github.com/Serendipity565/gora/internal/agent/eino"
	"github.com/Serendipity565/gora/internal/agent/llm"
	"github.com/Serendipity565/gora/internal/agent/tool"
	appconfig "github.com/Serendipity565/gora/internal/config"
	storage "github.com/Serendipity565/gora/internal/repository"
)

// historyCarrier 抽象出"能快照 / 恢复多 session 历史"的 Agent。
//
// EinoAgent 实现了它；存在这个接口主要是为了让历史读写助手不直接耦合 EinoAgent。
type historyCarrier interface {
	RestoreHistories(map[string][]*schema.Message)
	SnapshotHistories() map[string][]*schema.Message
}

// buildChatAgent 根据 cfg + llmIndex 创建一个 *eino.EinoAgent。
//
// previous 不为空时会复制其会话历史到新 Agent（用于切换模型时保留上下文）。
func buildChatAgent(
	ctx context.Context,
	cfg appconfig.Config,
	registry *tool.Registry,
	llmIndex int,
	previous *eino.EinoAgent,
) (*eino.EinoAgent, error) {
	modelConfig := cfg.LLM[llmIndex].ChatModelConfig()
	chatModel, err := llm.NewEinoModel(ctx, modelConfig)
	if err != nil {
		return nil, fmt.Errorf("初始化模型失败: %w", err)
	}

	agentConfig := eino.DefaultEinoConfig(chatModel, registry)
	agentConfig.Name = cfg.Agent.Name
	agentConfig.Description = cfg.Agent.Description
	agentConfig.Instruction = cfg.Agent.Instruction
	agentConfig.MaxHistoryMessages = cfg.Agent.MaxHistoryMessages
	agentConfig.MaxStreamChunkRunes = cfg.Agent.MaxStreamChunkRunes

	chatAgent, err := eino.NewEinoAgent(cfg.Agent.ID, agentConfig)
	if err != nil {
		return nil, err
	}
	if previous != nil {
		chatAgent.RestoreHistories(previous.SnapshotHistories())
	}

	return chatAgent, nil
}

// restoreModelSelection 从 modelStore 读出 (user, agent, session) 之前选择的 LLM 索引。
//
// 任何错误（包括读取失败 / 命中已不存在的模型）都会以 "回落到默认 0" 处理，
// 但会把诊断信息写到 errOut 提示用户。
func restoreModelSelection(
	ctx context.Context,
	out, errOut io.Writer,
	store storage.ModelSelectionStore,
	userID string,
	sessionID string,
	cfg appconfig.Config,
) int {
	selection, ok, err := store.Get(ctx, userID, cfg.Agent.ID, normalizeSessionID(sessionID))
	if err != nil {
		fmt.Fprintf(errOut, "读取模型选择失败，将使用默认模型: %v\n", err)
		return 0
	}
	if !ok {
		return 0
	}

	index, ok := resolveStoredLLMIndex(cfg, selection)
	if !ok {
		fmt.Fprintf(errOut, "已保存的模型 name 不在当前配置中，将使用默认模型: %s\n", selection.LLMName)
		return 0
	}

	fmt.Fprintf(out, "已恢复模型选择: %s\n", cfg.LLM[index].Name)
	return index
}

// saveModelSelection 把当前选择写入 modelStore。失败时只打印诊断，不阻塞主流程。
func saveModelSelection(
	ctx context.Context,
	errOut io.Writer,
	store storage.ModelSelectionStore,
	userID string,
	sessionID string,
	cfg appconfig.Config,
	llmIndex int,
) {
	if llmIndex < 0 || llmIndex >= len(cfg.LLM) {
		fmt.Fprintf(errOut, "保存模型选择失败: 模型序号超出范围: %d\n", llmIndex+1)
		return
	}

	llmConfig := cfg.LLM[llmIndex]
	err := store.Save(ctx, storage.ModelSelection{
		UserID:    userID,
		AgentID:   cfg.Agent.ID,
		SessionID: normalizeSessionID(sessionID),
		LLMName:   llmConfig.Name,
		Model:     llmConfig.Model,
		LLMIndex:  llmIndex,
	})
	if err != nil {
		fmt.Fprintf(errOut, "保存模型选择失败: %v\n", err)
	}
}

// resolveStoredLLMIndex 把存储中读到的 ModelSelection 翻译成当前 cfg.LLM 的下标。
//
// 唯一依据是 selection.LLMName —— 项目约定 name 是模型的唯一标识。
// 历史遗留的 Model / LLMIndex 字段已不参与查找，它们仅作为读时的诊断信息。
func resolveStoredLLMIndex(cfg appconfig.Config, selection storage.ModelSelection) (int, bool) {
	name := strings.TrimSpace(selection.LLMName)
	if name == "" {
		return 0, false
	}
	if index, _, err := cfg.FindLLM(name); err == nil {
		return index, true
	}
	return 0, false
}

// restoreConversationContext 优先用 Redis 短期记忆恢复上下文；命中失败回退到 MySQL 历史。
//
// 从 MySQL 拉到的历史会被回填到 Redis，避免下次重复 round-trip。
func restoreConversationContext(
	ctx context.Context,
	out, errOut io.Writer,
	historyStore storage.ChatHistoryStore,
	memoryStore storage.ActiveMemoryStore,
	userID string,
	sessionID string,
	cfg appconfig.Config,
	chatAgent historyCarrier,
) {
	sessionID = normalizeSessionID(sessionID)
	applyRestoredMessages := func(messages []storage.ChatMessage) {
		restored := chatMessagesToHistories(sessionID, messages)
		if len(restored) == 0 {
			return
		}
		histories := chatAgent.SnapshotHistories()
		if histories == nil {
			histories = make(map[string][]*schema.Message, len(restored))
		}
		for restoredSessionID, history := range restored {
			histories[restoredSessionID] = history
		}
		chatAgent.RestoreHistories(histories)
	}

	messages, ok, err := memoryStore.Get(ctx, userID, cfg.Agent.ID, sessionID)
	if err != nil {
		fmt.Fprintf(errOut, "读取短期记忆失败，将尝试读取数据库历史: %v\n", err)
	}
	if ok && len(messages) > 0 {
		applyRestoredMessages(messages)
		fmt.Fprintf(out, "已恢复短期记忆: %d 条消息\n", len(messages))
		return
	}

	messages, err = historyStore.ListRecentMessages(ctx, userID, cfg.Agent.ID, sessionID, cfg.Agent.MaxHistoryMessages)
	if err != nil {
		fmt.Fprintf(errOut, "读取历史对话失败，将从空上下文开始: %v\n", err)
		return
	}
	if len(messages) == 0 {
		return
	}

	applyRestoredMessages(messages)
	fmt.Fprintf(out, "已从历史记录恢复上下文: %d 条消息\n", len(messages))
	if err := memoryStore.Save(ctx, userID, cfg.Agent.ID, sessionID, messages); err != nil {
		fmt.Fprintf(errOut, "回填短期记忆失败: %v\n", err)
	}
}

// persistConversationTurn 把刚结束的一轮对话（user input + assistant reply）落到 MySQL，
// 并把当前 chatAgent 的全部 schema.Message 同步到 Redis 短期记忆。
//
// 任意一步失败都不会回滚，只把诊断写到 errOut。
func persistConversationTurn(
	ctx context.Context,
	errOut io.Writer,
	historyStore storage.ChatHistoryStore,
	memoryStore storage.ActiveMemoryStore,
	userID string,
	sessionID string,
	cfg appconfig.Config,
	llmIndex int,
	input, reply string,
	chatAgent historyCarrier,
) {
	sessionID = normalizeSessionID(sessionID)
	if llmIndex < 0 || llmIndex >= len(cfg.LLM) {
		fmt.Fprintf(errOut, "保存历史对话失败: 模型序号超出范围: %d\n", llmIndex+1)
		return
	}

	llmConfig := cfg.LLM[llmIndex]
	messages := []storage.ChatMessage{{
		UserID:    userID,
		AgentID:   cfg.Agent.ID,
		SessionID: sessionID,
		Role:      string(schema.User),
		Content:   input,
		LLMName:   llmConfig.Name,
		Model:     llmConfig.Model,
	}}
	if strings.TrimSpace(reply) != "" {
		messages = append(messages, storage.ChatMessage{
			UserID:    userID,
			AgentID:   cfg.Agent.ID,
			SessionID: sessionID,
			Role:      string(schema.Assistant),
			Content:   reply,
			LLMName:   llmConfig.Name,
			Model:     llmConfig.Model,
		})
	}
	if err := historyStore.AppendMessages(ctx, messages); err != nil {
		fmt.Fprintf(errOut, "保存历史对话失败: %v\n", err)
	}

	snapshot := chatAgent.SnapshotHistories()
	activeMessages := schemaMessagesToChatMessages(userID, cfg.Agent.ID, sessionID, llmConfig, snapshot[sessionID])
	activeMessages = trimChatMessages(activeMessages, cfg.Agent.MaxHistoryMessages)
	if err := memoryStore.Save(ctx, userID, cfg.Agent.ID, sessionID, activeMessages); err != nil {
		fmt.Fprintf(errOut, "保存短期记忆失败: %v\n", err)
	}
}

// chatMessagesToHistories 把 storage 里读出的消息转成 eino schema.Message，按 session 分组。
//
// 工具消息不需要回填给 LLM，直接丢弃；不识别 role 也丢弃。
func chatMessagesToHistories(sessionID string, messages []storage.ChatMessage) map[string][]*schema.Message {
	sessionID = normalizeSessionID(sessionID)
	history := make([]*schema.Message, 0, len(messages))
	for _, message := range messages {
		content := strings.TrimSpace(message.Content)
		if content == "" {
			continue
		}
		switch schema.RoleType(strings.ToLower(strings.TrimSpace(message.Role))) {
		case schema.User:
			history = append(history, schema.UserMessage(content))
		case schema.Assistant:
			history = append(history, schema.AssistantMessage(content, nil))
		case schema.System:
			history = append(history, schema.SystemMessage(content))
		}
	}
	if len(history) == 0 {
		return nil
	}
	return map[string][]*schema.Message{
		sessionID: history,
	}
}

// schemaMessagesToChatMessages 把 eino schema.Message 转成可落库的 storage.ChatMessage。
//
// Tool 消息不写入历史表（与 chatMessagesToHistories 对称）。
func schemaMessagesToChatMessages(
	userID, agentID, sessionID string,
	llmConfig appconfig.LLMConfig,
	messages []*schema.Message,
) []storage.ChatMessage {
	out := make([]storage.ChatMessage, 0, len(messages))
	for _, message := range messages {
		if message == nil {
			continue
		}
		content := strings.TrimSpace(message.Content)
		if content == "" {
			continue
		}
		role := strings.ToLower(strings.TrimSpace(string(message.Role)))
		if role != string(schema.User) && role != string(schema.Assistant) && role != string(schema.System) {
			continue
		}
		out = append(out, storage.ChatMessage{
			UserID:    userID,
			AgentID:   agentID,
			SessionID: sessionID,
			Role:      role,
			Content:   content,
			LLMName:   llmConfig.Name,
			Model:     llmConfig.Model,
		})
	}
	return out
}

// trimChatMessages 保留最后 max 条消息，超过 max 的从前面截断。
func trimChatMessages(messages []storage.ChatMessage, max int) []storage.ChatMessage {
	if max <= 0 || len(messages) <= max {
		return messages
	}
	return append([]storage.ChatMessage(nil), messages[len(messages)-max:]...)
}

// shortTermTTLOrFallback 用于 Run 在装配时计算短期记忆 TTL；这里复用 options.go 的解析。
//
// 之所以单独包一层，是想让 Run 内部少一行 if-err 噪音。
func shortTermTTLOrFallback(raw string, errOut io.Writer) time.Duration {
	ttl, err := shortTermMemoryTTL(raw)
	if err != nil {
		fmt.Fprintf(errOut, "短期记忆 TTL 配置无效，将使用 24h: %v\n", err)
		return 24 * time.Hour
	}
	return ttl
}
