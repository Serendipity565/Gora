package cmd

import (
	"context"

	appconfig "github.com/Serendipity565/gora/config"
	"github.com/spf13/cobra"
)

type cliOptions struct {
	ConfigPath          string
	APIKey              string
	BaseURL             string
	Model               string
	ServerAddr          string
	AgentID             string
	UserID              string
	DatabaseURL         string
	RedisAddr           string
	RedisPassword       string
	RedisDB             int
	ShortTermMemoryTTL  string
	MaxHistoryMessages  int
	MaxStreamChunkRunes int
}

var options = cliOptions{
	ConfigPath: appconfig.DefaultPath,
}

var rootCmd = &cobra.Command{
	Use:           "gora",
	Short:         "Gora Agent Web 服务（默认启动 Gin Web 服务，使用 `gora chat` 进入命令行对话）",
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE:          runServerCommand,
}

var chatCmd = &cobra.Command{
	Use:   "chat",
	Short: "启动交互式 Agent 命令行对话",
	RunE:  runChatCommand,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&options.ConfigPath, "config", options.ConfigPath, "Gora YAML 配置文件路径")
	rootCmd.PersistentFlags().StringVar(&options.APIKey, "api-key", "", "覆盖所有配置模型的 API Key；环境变量会按 provider 读取 DEEPSEEK_API_KEY 或 OPENAI_API_KEY")
	rootCmd.PersistentFlags().StringVar(&options.BaseURL, "base-url", "", "覆盖首个配置模型的 OpenAI 兼容接口地址")
	rootCmd.PersistentFlags().StringVar(&options.Model, "model", "", "覆盖首个配置模型的名称")
	rootCmd.PersistentFlags().StringVar(&options.AgentID, "agent-id", "", "覆盖配置中的 Agent ID")
	rootCmd.PersistentFlags().StringVar(&options.UserID, "user-id", "", "模型选择持久化使用的用户 ID，默认 local")
	rootCmd.PersistentFlags().StringVar(&options.DatabaseURL, "database-url", "", "覆盖配置中的 MySQL 连接串，默认也会读取 GORA_DATABASE_URL")
	rootCmd.PersistentFlags().StringVar(&options.RedisAddr, "redis-addr", "", "覆盖短期记忆 Redis 地址，默认也会读取 GORA_REDIS_ADDR")
	rootCmd.PersistentFlags().StringVar(&options.RedisPassword, "redis-password", "", "覆盖短期记忆 Redis 密码，默认也会读取 GORA_REDIS_PASSWORD")
	rootCmd.PersistentFlags().IntVar(&options.RedisDB, "redis-db", -1, "覆盖短期记忆 Redis DB，默认也会读取 GORA_REDIS_DB")
	rootCmd.PersistentFlags().StringVar(&options.ShortTermMemoryTTL, "short-term-memory-ttl", "", "覆盖 Redis 短期记忆过期时间，例如 24h，默认也会读取 GORA_SHORT_TERM_MEMORY_TTL")
	rootCmd.PersistentFlags().IntVar(&options.MaxHistoryMessages, "max-history", 0, "覆盖配置中的历史消息保留数量")
	rootCmd.PersistentFlags().IntVar(&options.MaxStreamChunkRunes, "max-chunk-runes", 0, "覆盖配置中的单个流式输出事件字符数")
	rootCmd.Flags().StringVar(&options.ServerAddr, "addr", ":8080", "Gin Web 服务监听地址")

	rootCmd.AddCommand(chatCmd)
}

func Execute() error {
	if rootCmd.Context() == nil {
		rootCmd.SetContext(context.Background())
	}
	return rootCmd.Execute()
}
