package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/spf13/cobra"

	"github.com/Serendipity565/gora/agent"
	appconfig "github.com/Serendipity565/gora/config"
	"github.com/Serendipity565/gora/llm"
	"github.com/Serendipity565/gora/storage"
	"github.com/Serendipity565/gora/tool"
	"github.com/Serendipity565/gora/tool/builtin"
)

func runChatCommand(command *cobra.Command, args []string) error {
	return runChat(command.Context(), command.InOrStdin(), command.OutOrStdout(), command.ErrOrStderr(), options)
}

func runChat(parent context.Context, in io.Reader, out, errOut io.Writer, opts cliOptions) error {
	if parent == nil {
		parent = context.Background()
	}

	fmt.Fprintln(out, "╔══════════════════════════════════╗")
	fmt.Fprintln(out, "║        Gora Agent CLI           ║")
	fmt.Fprintln(out, "║   Goroutine + Agent = Gora      ║")
	fmt.Fprintln(out, "╚══════════════════════════════════╝")
	fmt.Fprintln(out)

	cfg, err := appconfig.Read(opts.ConfigPath)
	if err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}
	applyRuntimeOverrides(&cfg, opts)

	scanner := bufio.NewScanner(in)
	if err := fillMissingAPIKeys(out, scanner, &cfg); err != nil {
		return fmt.Errorf("读取 API Key 失败: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("配置校验失败: %w", err)
	}

	registry := tool.NewRegistry()
	if err := registry.Register(builtin.NewHTTPTool()); err != nil {
		return fmt.Errorf("注册工具失败: %w", err)
	}
	fmt.Fprintf(out, "✅ 已加载 %d 个工具\n", len(registry.List()))

	userID := modelSelectionUserID(opts)
	modelStore, err := storage.OpenModelSelectionStore(parent, cfg.DatabaseURL())
	if err != nil {
		fmt.Fprintf(errOut, "模型选择存储不可用，本次会话内仍可切换模型: %v\n", err)
		modelStore = storage.NoopModelSelectionStore{}
	}
	defer func() {
		if err := modelStore.Close(); err != nil {
			fmt.Fprintf(errOut, "关闭模型选择存储失败: %v\n", err)
		}
	}()

	redisConfig := cfg.RedisSettings()
	memoryTTL, err := shortTermMemoryTTL(redisConfig.ShortTermTTL)
	if err != nil {
		fmt.Fprintf(errOut, "短期记忆 TTL 配置无效，将使用 24h: %v\n", err)
		memoryTTL = 24 * time.Hour
	}
	memoryStore, err := storage.OpenActiveMemoryStore(parent, redisConfig.Addr, redisConfig.Password, redisConfig.DB, memoryTTL)
	if err != nil {
		fmt.Fprintf(errOut, "短期记忆 Redis 不可用，将仅使用数据库历史: %v\n", err)
		memoryStore = storage.NoopActiveMemoryStore{}
	}
	defer func() {
		if err := memoryStore.Close(); err != nil {
			fmt.Fprintf(errOut, "关闭短期记忆存储失败: %v\n", err)
		}
	}()

	currentLLMIndex := 0
	if strings.TrimSpace(opts.Model) == "" {
		currentLLMIndex = restoreModelSelection(parent, out, errOut, modelStore, userID, cfg)
	}
	myAgent, err := buildChatAgent(parent, cfg, registry, currentLLMIndex, nil)
	if err != nil {
		return fmt.Errorf("创建 Eino Agent 失败: %w", err)
	}
	restoreConversationContext(parent, out, errOut, modelStore, memoryStore, userID, cfg, myAgent)

	fmt.Fprintf(out, "🤖 %s 已就绪 (模型: %s, 配置: %s)\n", myAgent.ID(), cfg.LLM[currentLLMIndex].DisplayName(currentLLMIndex), opts.ConfigPath)
	fmt.Fprintln(out, "输入消息与 Agent 对话，输入 /model 查看或切换模型，输入 /quit 退出")
	fmt.Fprintln(out, strings.Repeat("─", 50))

	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)
	go func() {
		select {
		case <-sigCh:
			fmt.Fprintln(out, "\n👋 正在退出...")
			cancel()
		case <-ctx.Done():
		}
	}()

	for {
		fmt.Fprint(out, "\n💬 你: ")
		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}
		if input == "/quit" {
			fmt.Fprintln(out, "👋 再见！")
			break
		}
		if strings.HasPrefix(input, "/model") {
			nextIndex, nextAgent, handled, err := handleModelCommand(parent, out, cfg, registry, myAgent, currentLLMIndex, input)
			if err != nil {
				fmt.Fprintf(errOut, "模型切换失败: %v\n", err)
				continue
			}
			if handled {
				currentLLMIndex = nextIndex
				myAgent = nextAgent
				saveModelSelection(parent, errOut, modelStore, userID, cfg, currentLLMIndex)
			}
			continue
		}

		fmt.Fprintf(out, "\n🤖 %s:\n", myAgent.ID())

		events := myAgent.Run(ctx, input)
		var assistantReply strings.Builder
		var gotDone, gotError bool
		for event := range events {
			renderEvent(out, event)
			switch event.Type {
			case agent.EventChunk:
				assistantReply.WriteString(event.Content)
			case agent.EventDone:
				gotDone = true
			case agent.EventError:
				gotError = true
			}
		}
		if gotDone && !gotError {
			persistConversationTurn(parent, errOut, modelStore, memoryStore, userID, cfg, currentLLMIndex, input, assistantReply.String(), myAgent)
		}

		fmt.Fprintln(out)
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(errOut, "读取输入失败: %v\n", err)
	}

	return nil
}

func buildChatAgent(
	ctx context.Context,
	cfg appconfig.Config,
	registry *tool.Registry,
	llmIndex int,
	previous *agent.EinoAgent,
) (*agent.EinoAgent, error) {
	modelConfig := cfg.LLM[llmIndex].ChatModelConfig()
	chatModel, err := llm.NewOpenAICompatibleEinoModel(ctx, modelConfig)
	if err != nil {
		return nil, fmt.Errorf("初始化模型失败: %w", err)
	}

	agentConfig := agent.DefaultEinoConfig(chatModel, registry)
	agentConfig.Name = cfg.Agent.Name
	agentConfig.Description = cfg.Agent.Description
	agentConfig.Instruction = cfg.Agent.Instruction
	agentConfig.MaxHistoryMessages = cfg.Agent.MaxHistoryMessages
	agentConfig.MaxStreamChunkRunes = cfg.Agent.MaxStreamChunkRunes

	chatAgent, err := agent.NewEinoAgent(cfg.Agent.ID, agentConfig)
	if err != nil {
		return nil, err
	}
	if previous != nil {
		chatAgent.RestoreHistories(previous.SnapshotHistories())
	}

	return chatAgent, nil
}

func handleModelCommand(
	ctx context.Context,
	out io.Writer,
	cfg appconfig.Config,
	registry *tool.Registry,
	currentAgent *agent.EinoAgent,
	currentLLMIndex int,
	input string,
) (int, *agent.EinoAgent, bool, error) {
	selector := strings.TrimSpace(strings.TrimPrefix(input, "/model"))
	if selector == "" || strings.EqualFold(selector, "list") {
		renderModelList(out, cfg, currentLLMIndex)
		return currentLLMIndex, currentAgent, false, nil
	}

	nextIndex, target, err := cfg.FindLLM(selector)
	if err != nil {
		return currentLLMIndex, currentAgent, false, err
	}
	if nextIndex == currentLLMIndex {
		fmt.Fprintf(out, "当前已使用模型: %s\n", target.DisplayName(nextIndex))
		return currentLLMIndex, currentAgent, false, nil
	}

	nextAgent, err := buildChatAgent(ctx, cfg, registry, nextIndex, currentAgent)
	if err != nil {
		return currentLLMIndex, currentAgent, false, err
	}

	fmt.Fprintf(out, "已切换到模型: %s\n", target.DisplayName(nextIndex))
	return nextIndex, nextAgent, true, nil
}

func renderModelList(out io.Writer, cfg appconfig.Config, currentLLMIndex int) {
	fmt.Fprintln(out, "可用模型:")
	for index, llmConfig := range cfg.LLM {
		marker := " "
		if index == currentLLMIndex {
			marker = "*"
		}
		fmt.Fprintf(out, "  %s %d. %s\n", marker, index+1, llmConfig.DisplayName(index))
	}
	fmt.Fprintln(out, "使用 /model <序号|name|model> 切换")
}

func restoreModelSelection(
	ctx context.Context,
	out, errOut io.Writer,
	store storage.ModelSelectionStore,
	userID string,
	cfg appconfig.Config,
) int {
	selection, ok, err := store.Get(ctx, userID, cfg.Agent.ID, agent.DefaultSessionID)
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
		SessionID: agent.DefaultSessionID,
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
	cfg appconfig.Config,
	chatAgent *agent.EinoAgent,
) {
	messages, ok, err := memoryStore.Get(ctx, userID, cfg.Agent.ID, agent.DefaultSessionID)
	if err != nil {
		fmt.Fprintf(errOut, "读取短期记忆失败，将尝试读取数据库历史: %v\n", err)
	}
	if ok && len(messages) > 0 {
		chatAgent.RestoreHistories(chatMessagesToHistories(messages))
		fmt.Fprintf(out, "已恢复短期记忆: %d 条消息\n", len(messages))
		return
	}

	messages, err = historyStore.ListRecentMessages(ctx, userID, cfg.Agent.ID, agent.DefaultSessionID, cfg.Agent.MaxHistoryMessages)
	if err != nil {
		fmt.Fprintf(errOut, "读取历史对话失败，将从空上下文开始: %v\n", err)
		return
	}
	if len(messages) == 0 {
		return
	}

	chatAgent.RestoreHistories(chatMessagesToHistories(messages))
	fmt.Fprintf(out, "已从历史记录恢复上下文: %d 条消息\n", len(messages))
	if err := memoryStore.Save(ctx, userID, cfg.Agent.ID, agent.DefaultSessionID, messages); err != nil {
		fmt.Fprintf(errOut, "回填短期记忆失败: %v\n", err)
	}
}

func persistConversationTurn(
	ctx context.Context,
	errOut io.Writer,
	historyStore storage.ChatHistoryStore,
	memoryStore storage.ActiveMemoryStore,
	userID string,
	cfg appconfig.Config,
	llmIndex int,
	input, reply string,
	chatAgent *agent.EinoAgent,
) {
	if llmIndex < 0 || llmIndex >= len(cfg.LLM) {
		fmt.Fprintf(errOut, "保存历史对话失败: 模型序号超出范围: %d\n", llmIndex+1)
		return
	}

	llmConfig := cfg.LLM[llmIndex]
	messages := []storage.ChatMessage{{
		UserID:    userID,
		AgentID:   cfg.Agent.ID,
		SessionID: agent.DefaultSessionID,
		Role:      string(schema.User),
		Content:   input,
		LLMName:   llmConfig.Name,
		Model:     llmConfig.Model,
	}}
	if strings.TrimSpace(reply) != "" {
		messages = append(messages, storage.ChatMessage{
			UserID:    userID,
			AgentID:   cfg.Agent.ID,
			SessionID: agent.DefaultSessionID,
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
	activeMessages := schemaMessagesToChatMessages(userID, cfg.Agent.ID, agent.DefaultSessionID, llmConfig, snapshot[agent.DefaultSessionID])
	activeMessages = trimChatMessages(activeMessages, cfg.Agent.MaxHistoryMessages)
	if err := memoryStore.Save(ctx, userID, cfg.Agent.ID, agent.DefaultSessionID, activeMessages); err != nil {
		fmt.Fprintf(errOut, "保存短期记忆失败: %v\n", err)
	}
}

func chatMessagesToHistories(messages []storage.ChatMessage) map[string][]*schema.Message {
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
		agent.DefaultSessionID: history,
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

func fillMissingAPIKeys(out io.Writer, scanner *bufio.Scanner, cfg *appconfig.Config) error {
	for index := range cfg.LLM {
		if strings.TrimSpace(cfg.LLM[index].APIKey) != "" {
			continue
		}

		provider := strings.ToUpper(strings.TrimSpace(cfg.LLM[index].Provider))
		if provider == "" {
			provider = "LLM"
		}

		fmt.Fprintf(out, "请输入 %s 模型 %s 的 API Key: ", provider, cfg.LLM[index].DisplayName(index))
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return err
			}
			return io.EOF
		}

		cfg.LLM[index].APIKey = strings.TrimSpace(scanner.Text())
	}
	return nil
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

func renderEvent(out io.Writer, event agent.Event) {
	switch event.Type {
	case agent.EventThinking:
		fmt.Fprintf(out, "  🧠 %s\n", event.Content)
	case agent.EventToolCall:
		fmt.Fprintf(out, "  🔧 调用工具: %s\n", event.Content)
		if args, ok := event.Metadata["args"]; ok {
			fmt.Fprintf(out, "     参数: %s\n", jsonMarshal(args))
		}
	case agent.EventToolResult:
		fmt.Fprintf(out, "  📥 工具返回: %s\n", truncateRunes(event.Content, 200))
	case agent.EventChunk:
		fmt.Fprint(out, event.Content)
	case agent.EventDone:
		fmt.Fprintln(out)
	case agent.EventError:
		fmt.Fprintf(out, "\n  ❌ 错误: %s\n", event.Content)
	}
}

func jsonMarshal(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

func truncateRunes(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "..."
}
