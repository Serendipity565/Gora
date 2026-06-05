package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"

	"github.com/Serendipity565/gora/internal/agent/eino"
	"github.com/Serendipity565/gora/internal/agent/llm"
	"github.com/Serendipity565/gora/internal/agent/tool"
	appconfig "github.com/Serendipity565/gora/internal/config"
	storage "github.com/Serendipity565/gora/internal/repository"
)

type historyCarrier interface {
	RestoreHistories(map[string][]*schema.Message)
	SnapshotHistories() map[string][]*schema.Message
}

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
		fmt.Fprintf(errOut, "已保存的模型选择不在当前配置中，将使用默认模型: %s\n", selection.Model)
		return 0
	}

	fmt.Fprintf(out, "已恢复模型选择: %s\n", cfg.LLM[index].DisplayName(index))
	return index
}

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

func resolveStoredLLMIndex(cfg appconfig.Config, selection storage.ModelSelection) (int, bool) {
	if strings.TrimSpace(selection.LLMName) != "" {
		if index, _, err := cfg.FindLLM(selection.LLMName); err == nil {
			return index, true
		}
	}
	if strings.TrimSpace(selection.Model) != "" {
		if index, _, err := cfg.FindLLM(selection.Model); err == nil {
			return index, true
		}
	}
	if selection.LLMIndex >= 0 && selection.LLMIndex < len(cfg.LLM) {
		llmConfig := cfg.LLM[selection.LLMIndex]
		if selection.Model == "" || strings.EqualFold(llmConfig.Model, selection.Model) || strings.EqualFold(llmConfig.Name, selection.LLMName) {
			return selection.LLMIndex, true
		}
	}
	return 0, false
}

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

func trimChatMessages(messages []storage.ChatMessage, max int) []storage.ChatMessage {
	if max <= 0 || len(messages) <= max {
		return messages
	}
	return append([]storage.ChatMessage(nil), messages[len(messages)-max:]...)
}

func normalizeSessionID(sessionID string) string {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return eino.DefaultSessionID
	}
	return sessionID
}

func applyRuntimeOverrides(cfg *appconfig.Config, opts cliOptions) {
	applyProviderAPIKeyEnv(cfg, "DEEPSEEK_API_KEY", "deepseek")
	applyProviderAPIKeyEnv(cfg, "OPENAI_API_KEY", "openai")
	if strings.TrimSpace(opts.APIKey) != "" {
		for index := range cfg.LLM {
			cfg.LLM[index].APIKey = strings.TrimSpace(opts.APIKey)
		}
	}
	if strings.TrimSpace(opts.BaseURL) != "" && len(cfg.LLM) > 0 {
		cfg.LLM[0].BaseURL = strings.TrimSpace(opts.BaseURL)
	}
	if strings.TrimSpace(opts.Model) != "" && len(cfg.LLM) > 0 {
		cfg.LLM[0].Model = strings.TrimSpace(opts.Model)
	}
	if strings.TrimSpace(opts.AgentID) != "" {
		cfg.Agent.ID = strings.TrimSpace(opts.AgentID)
	}
	if envDatabaseURL := strings.TrimSpace(os.Getenv("GORA_DATABASE_URL")); envDatabaseURL != "" && strings.TrimSpace(cfg.Database.URL) == "" {
		cfg.Database.URL = envDatabaseURL
	}
	if strings.TrimSpace(opts.DatabaseURL) != "" {
		cfg.Database.URL = strings.TrimSpace(opts.DatabaseURL)
	}
	if envRedisAddr := strings.TrimSpace(os.Getenv("GORA_REDIS_ADDR")); envRedisAddr != "" {
		cfg.Database.Redis.Addr = envRedisAddr
		cfg.Redis.Addr = envRedisAddr
	}
	if strings.TrimSpace(opts.RedisAddr) != "" {
		cfg.Database.Redis.Addr = strings.TrimSpace(opts.RedisAddr)
		cfg.Redis.Addr = strings.TrimSpace(opts.RedisAddr)
	}
	if envRedisPassword := strings.TrimSpace(os.Getenv("GORA_REDIS_PASSWORD")); envRedisPassword != "" {
		cfg.Database.Redis.Password = envRedisPassword
		cfg.Redis.Password = envRedisPassword
	}
	if strings.TrimSpace(opts.RedisPassword) != "" {
		cfg.Database.Redis.Password = strings.TrimSpace(opts.RedisPassword)
		cfg.Redis.Password = strings.TrimSpace(opts.RedisPassword)
	}
	if envRedisDB := strings.TrimSpace(os.Getenv("GORA_REDIS_DB")); envRedisDB != "" {
		if redisDB, err := strconv.Atoi(envRedisDB); err == nil {
			cfg.Database.Redis.DB = redisDB
			cfg.Redis.DB = redisDB
		}
	}
	if opts.RedisDB >= 0 {
		cfg.Database.Redis.DB = opts.RedisDB
		cfg.Redis.DB = opts.RedisDB
	}
	if envMemoryTTL := strings.TrimSpace(os.Getenv("GORA_SHORT_TERM_MEMORY_TTL")); envMemoryTTL != "" {
		cfg.Database.Redis.ShortTermTTL = envMemoryTTL
		cfg.Redis.ShortTermTTL = envMemoryTTL
	}
	if strings.TrimSpace(opts.ShortTermMemoryTTL) != "" {
		cfg.Database.Redis.ShortTermTTL = strings.TrimSpace(opts.ShortTermMemoryTTL)
		cfg.Redis.ShortTermTTL = strings.TrimSpace(opts.ShortTermMemoryTTL)
	}
	if opts.MaxHistoryMessages > 0 {
		cfg.Agent.MaxHistoryMessages = opts.MaxHistoryMessages
	}
	if opts.MaxStreamChunkRunes > 0 {
		cfg.Agent.MaxStreamChunkRunes = opts.MaxStreamChunkRunes
	}
}

func applyProviderAPIKeyEnv(cfg *appconfig.Config, envName, provider string) {
	apiKey := strings.TrimSpace(os.Getenv(envName))
	if apiKey == "" {
		return
	}
	for index := range cfg.LLM {
		if !strings.EqualFold(strings.TrimSpace(cfg.LLM[index].Provider), provider) {
			continue
		}
		if strings.TrimSpace(cfg.LLM[index].APIKey) == "" {
			cfg.LLM[index].APIKey = apiKey
		}
	}
}

func shortTermMemoryTTL(raw string) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 24 * time.Hour, nil
	}
	return time.ParseDuration(raw)
}

func modelSelectionUserID(opts cliOptions) string {
	if userID := strings.TrimSpace(opts.UserID); userID != "" {
		return userID
	}
	if userID := strings.TrimSpace(os.Getenv("GORA_USER_ID")); userID != "" {
		return userID
	}
	return storage.LocalUserID
}
