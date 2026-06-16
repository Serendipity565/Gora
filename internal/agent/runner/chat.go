package runner

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	appconfig "github.com/Serendipity565/gora/configs"
	"github.com/cloudwego/eino/schema"

	"github.com/Serendipity565/gora/internal/agent/eino"
	"github.com/Serendipity565/gora/internal/agent/llm"
	"github.com/Serendipity565/gora/internal/agent/tool"
	"github.com/Serendipity565/gora/internal/repository"
	"github.com/Serendipity565/gora/internal/repository/model"
	"github.com/Serendipity565/gora/pkg/logger"
)

// defaultTailTimeout 是 trailing 异步操作（落库 / 短期记忆刷新）的默认超时。
//
// 派生 tailContext 时使用；语义见 Runner.tailContext。
const defaultTailTimeout = 10 * time.Second

// tailContext 派生一个不再受 parent cancel 影响的 ctx，并叠一层独立超时。
//
// 应用场景：HTTP/SSE 请求结束后仍需要继续做的"对话尾声"工作，比如把会话落库、
// 把短期记忆刷回 Redis。controller 在 SSE 结束的瞬间会触发 defer cancel()，
// 直接复用请求 ctx 会让所有 trailing DAO 调用立刻因 ctx canceled 静默失败；
// 这里通过 context.WithoutCancel 派生新 ctx，再叠 defaultTailTimeout 兜底。
//
// 调用方拿到 cancel 后必须保证调用——一般 defer cancel()。
func tailContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(parent), defaultTailTimeout)
}

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
//
// log 用于把"短期记忆 miss / 数据库读取失败"等诊断信息推到项目日志体系，
// 不会用 panic / return 干扰主流程——任何一步失败都只记日志、继续走兜底路径。
func restoreConversationContext(
	ctx context.Context,
	log logger.Logger,
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
		log.Warn("读取短期记忆失败，将尝试读取数据库历史",
			logger.String("session_id", sessionID),
			logger.Uint64("user_id", userID),
			logger.String("error", err.Error()),
		)
	}
	if ok && len(cached) > 0 {
		applyRestoredMessages(cached)
		log.Info("已恢复短期记忆",
			logger.String("session_id", sessionID),
			logger.Int("messages", len(cached)),
		)
		return
	}

	persisted, err := messageDAO.ListRecent(ctx, sessionID, cfg.Agent.MaxHistoryMessages)
	if err != nil {
		log.Warn("读取历史对话失败，将从空上下文开始",
			logger.String("session_id", sessionID),
			logger.String("error", err.Error()),
		)
		return
	}
	if len(persisted) == 0 {
		return
	}

	applyRestoredMessages(persisted)
	log.Info("已从历史记录恢复上下文",
		logger.String("session_id", sessionID),
		logger.Int("messages", len(persisted)),
	)
	if err := memoryStore.Save(ctx, userID, cfg.Agent.ID, sessionID, persisted); err != nil {
		log.Warn("回填短期记忆失败",
			logger.String("session_id", sessionID),
			logger.String("error", err.Error()),
		)
	}
}

// persistConversationTurn 把刚结束的一轮对话的全部消息落 MySQL（user、thinking、
// tool_call、tool_result、assistant reply），并同步 chatAgent 的 schema.Message
// 到 Redis 短期记忆。
//
// turn 由调用方按时间顺序构造：调用方负责填好 Role / Content / ToolCalls，
// 本函数负责补齐 SessionID / Seq / UserID / AgentID / LLM / Model 并成批写入。
//
// 任意一步失败都不会回滚，只把诊断推给 log。
//
// 调用约定：本函数应运行在 tailContext 派生的 ctx 上——即不再受请求 cancel
// 影响、且自带独立超时；否则 SSE 一结束 controller 触发 cancel 会让所有 DAO
// 调用立刻 ctx canceled 失败。详见 tailContext 注释。
func persistConversationTurn(
	ctx context.Context,
	log logger.Logger,
	sessionDAO repository.SessionDAO,
	messageDAO repository.MessageDAO,
	memoryStore repository.ActiveMemoryStore,
	userID uint64,
	sessionID string,
	cfg appconfig.Config,
	llmIndex int,
	turn []model.Message,
	chatAgent historyCarrier,
	now func() time.Time,
) {
	if len(turn) == 0 {
		return
	}
	if llmIndex < 0 || llmIndex >= len(cfg.LLM) {
		log.Error("保存历史对话失败: 模型序号超出范围",
			logger.String("session_id", sessionID),
			logger.Int("llm_index", llmIndex),
		)
		return
	}

	llmConfig := cfg.LLM[llmIndex]

	// 取当前 session 的下一个 seq，所有本回合消息连续编号。
	seq, err := messageDAO.NextSeq(ctx, sessionID)
	if err != nil {
		log.Error("读取消息序号失败",
			logger.String("session_id", sessionID),
			logger.String("error", err.Error()),
		)
		return
	}

	for i := range turn {
		turn[i].SessionID = sessionID
		turn[i].Seq = seq + uint64(i)
		turn[i].UserID = userID
		turn[i].AgentID = cfg.Agent.ID
		turn[i].LLMName = llmConfig.Name
		turn[i].Model = llmConfig.Model
	}
	if err := messageDAO.Append(ctx, turn); err != nil {
		log.Error("保存历史对话失败",
			logger.String("session_id", sessionID),
			logger.String("error", err.Error()),
		)
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
		log.Error("更新会话元信息失败",
			logger.String("session_id", sessionID),
			logger.String("error", err.Error()),
		)
	}

	// 同步刷新 Redis 短期记忆（snapshot 截断到 max）。
	snapshot := chatAgent.SnapshotHistories()
	activeMessages := schemaMessagesToMessages(userID, cfg.Agent.ID, sessionID, llmConfig, snapshot[sessionID])
	activeMessages = trimMessages(activeMessages, cfg.Agent.MaxHistoryMessages)
	if err := memoryStore.Save(ctx, userID, cfg.Agent.ID, sessionID, activeMessages); err != nil {
		log.Warn("保存短期记忆失败",
			logger.String("session_id", sessionID),
			logger.String("error", err.Error()),
		)
	}
}

// messagesToHistories 把 model.Message 转成 eino schema.Message，按 session 分组。
//
// 工具消息（RoleTool）会被回填为 schema.ToolMessage，以保证恢复后的
// Eino Agent 能正确处理工具调用上下文。
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
		case schema.Tool:
			// Tool call 消息附带 args（ToolCalls JSON 列），
			// Tool result 消息仅包含结果文本，统一以 ToolMessage 回填。
			history = append(history, schema.ToolMessage(content, ""))
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
		case string(schema.Tool):
			typed = model.RoleTool
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
