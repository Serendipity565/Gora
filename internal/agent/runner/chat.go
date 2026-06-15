package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	appconfig "github.com/Serendipity565/gora/configs"
	"github.com/cloudwego/eino/schema"

	"github.com/Serendipity565/gora/internal/agent/eino"
	"github.com/Serendipity565/gora/internal/agent/llm"
	"github.com/Serendipity565/gora/internal/agent/tool"
	"github.com/Serendipity565/gora/internal/repository"
	"github.com/Serendipity565/gora/internal/repository/model"
)

// historyCarrier 抽象出"能快照 / 恢复多 session 历史"的 Agent。
type historyCarrier interface {
	RestoreHistories(map[string][]*schema.Message)
	SnapshotHistories() map[string][]*schema.Message
}

// llmConfigToChatModelConfig 将配置层的 LLMConfig 转换为 LLM 层可用的 ChatModelConfig。
func llmConfigToChatModelConfig(c appconfig.LLMConfig) llm.ChatModelConfig {
	return llm.ChatModelConfig{
		APIStyle: strings.ToLower(strings.TrimSpace(c.APIStyle)),
		APIKey:   strings.TrimSpace(c.APIKey),
		BaseURL:  strings.TrimSpace(c.BaseURL),
		Model:    strings.TrimSpace(c.Model),
	}
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
	modelConfig := llmConfigToChatModelConfig(cfg.LLM[llmIndex])
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

// resolveStoredLLMIndex 把 session 中持久化的 llm_name 翻译成当前 cfg.LLM 的下标。
//
// 唯一依据是 LLMName —— 项目约定 name 是模型的唯一标识。
func resolveStoredLLMIndex(cfg appconfig.Config, llmName string) (int, bool) {
	name := strings.TrimSpace(llmName)
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
	messageDAO repository.MessageDAO,
	memoryStore repository.ActiveMemoryStore,
	userID uint64,
	sessionID string,
	cfg appconfig.Config,
	chatAgent historyCarrier,
) {
	applyRestoredMessages := func(messages []model.Message) {
		restored := messagesToHistories(sessionID, messages)
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

	cached, ok, err := memoryStore.Get(ctx, userID, cfg.Agent.ID, sessionID)
	if err != nil {
		fmt.Fprintf(errOut, "读取短期记忆失败，将尝试读取数据库历史: %v\n", err)
	}
	if ok && len(cached) > 0 {
		applyRestoredMessages(cached)
		fmt.Fprintf(out, "已恢复短期记忆: %d 条消息\n", len(cached))
		return
	}

	persisted, err := messageDAO.ListRecent(ctx, sessionID, cfg.Agent.MaxHistoryMessages)
	if err != nil {
		fmt.Fprintf(errOut, "读取历史对话失败，将从空上下文开始: %v\n", err)
		return
	}
	if len(persisted) == 0 {
		return
	}

	applyRestoredMessages(persisted)
	fmt.Fprintf(out, "已从历史记录恢复上下文: %d 条消息\n", len(persisted))
	if err := memoryStore.Save(ctx, userID, cfg.Agent.ID, sessionID, persisted); err != nil {
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
	sessionDAO repository.SessionDAO,
	messageDAO repository.MessageDAO,
	memoryStore repository.ActiveMemoryStore,
	userID uint64,
	sessionID string,
	cfg appconfig.Config,
	llmIndex int,
	input, reply string,
	chatAgent historyCarrier,
	now func() time.Time,
) {
	if llmIndex < 0 || llmIndex >= len(cfg.LLM) {
		fmt.Fprintf(errOut, "保存历史对话失败: 模型序号超出范围: %d\n", llmIndex+1)
		return
	}

	llmConfig := cfg.LLM[llmIndex]

	// 取下一个 seq 起点。
	seq, err := messageDAO.NextSeq(ctx, sessionID)
	if err != nil {
		fmt.Fprintf(errOut, "读取消息序号失败: %v\n", err)
		return
	}

	turn := []model.Message{{
		SessionID: sessionID,
		Seq:       seq,
		UserID:    userID,
		AgentID:   cfg.Agent.ID,
		Role:      model.RoleUser,
		Content:   input,
		LLMName:   llmConfig.Name,
		Model:     llmConfig.Model,
	}}
	if strings.TrimSpace(reply) != "" {
		turn = append(turn, model.Message{
			SessionID: sessionID,
			Seq:       seq + 1,
			UserID:    userID,
			AgentID:   cfg.Agent.ID,
			Role:      model.RoleAssistant,
			Content:   reply,
			LLMName:   llmConfig.Name,
			Model:     llmConfig.Model,
		})
	}
	if err := messageDAO.Append(ctx, turn); err != nil {
		fmt.Fprintf(errOut, "保存历史对话失败: %v\n", err)
	}

	// 同步刷新 session 元信息（LLM、last_message_at、归属用户）。
	if err := sessionDAO.Upsert(ctx, &model.Session{
		ID:            sessionID,
		UserID:        userID,
		Title:         "新对话",
		LLMName:       llmConfig.Name,
		LastMessageAt: now(),
		Status:        model.SessionStatusActive,
	}); err != nil {
		fmt.Fprintf(errOut, "更新会话元信息失败: %v\n", err)
	}

	// 同步刷新 Redis 短期记忆（snapshot 截断到 max）。
	snapshot := chatAgent.SnapshotHistories()
	activeMessages := schemaMessagesToMessages(userID, cfg.Agent.ID, sessionID, llmConfig, snapshot[sessionID])
	activeMessages = trimMessages(activeMessages, cfg.Agent.MaxHistoryMessages)
	if err := memoryStore.Save(ctx, userID, cfg.Agent.ID, sessionID, activeMessages); err != nil {
		fmt.Fprintf(errOut, "保存短期记忆失败: %v\n", err)
	}
}

// messagesToHistories 把 model.Message 转成 eino schema.Message，按 session 分组。
//
// 工具消息不需要回填给 LLM，直接丢弃；不识别 role 也丢弃。
func messagesToHistories(sessionID string, messages []model.Message) map[string][]*schema.Message {
	if strings.TrimSpace(sessionID) == "" {
		return nil
	}
	history := make([]*schema.Message, 0, len(messages))
	for _, message := range messages {
		content := strings.TrimSpace(message.Content)
		if content == "" {
			continue
		}
		switch schema.RoleType(strings.ToLower(strings.TrimSpace(string(message.Role)))) {
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

// schemaMessagesToMessages 把 eino schema.Message 转成可缓存的 model.Message。
//
// Tool 消息不写入历史表（与 messagesToHistories 对称）。
func schemaMessagesToMessages(
	userID uint64, agentID, sessionID string,
	llmConfig appconfig.LLMConfig,
	messages []*schema.Message,
) []model.Message {
	out := make([]model.Message, 0, len(messages))
	for _, message := range messages {
		if message == nil {
			continue
		}
		content := strings.TrimSpace(message.Content)
		if content == "" {
			continue
		}
		role := strings.ToLower(strings.TrimSpace(string(message.Role)))
		var typed model.MessageRole
		switch role {
		case string(schema.User):
			typed = model.RoleUser
		case string(schema.Assistant):
			typed = model.RoleAssistant
		case string(schema.System):
			typed = model.RoleSystem
		default:
			continue
		}
		out = append(out, model.Message{
			SessionID: sessionID,
			UserID:    userID,
			AgentID:   agentID,
			Role:      typed,
			Content:   content,
			LLMName:   llmConfig.Name,
			Model:     llmConfig.Model,
		})
	}
	return out
}

// trimMessages 保留最后 max 条消息，超过 max 的从前面截断。
func trimMessages(messages []model.Message, max int) []model.Message {
	if max <= 0 || len(messages) <= max {
		return messages
	}
	return append([]model.Message(nil), messages[len(messages)-max:]...)
}

// requireSessionID 校验 sessionID 非空；空时返回错误。
//
// 项目去掉了 "default" 兜底语义：每次对话/查询都必须显式给出 session id。
func requireSessionID(sessionID string) (string, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return "", errors.New("session id 不能为空")
	}
	return sessionID, nil
}
