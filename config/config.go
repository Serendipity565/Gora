package config

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Serendipity565/gora/llm"
	"github.com/spf13/viper"
)

// DefaultPath 是 Gora CLI 默认读取的配置文件路径。
const DefaultPath = "config/config.yaml"

// Config 是应用启动配置。
type Config struct {
	LLM      []LLMConfig    `mapstructure:"llm" yaml:"llm"`
	Agent    AgentConfig    `mapstructure:"agent" yaml:"agent"`
	Database DatabaseConfig `mapstructure:"database" yaml:"database"`
	Redis    RedisConfig    `mapstructure:"redis" yaml:"redis"`
}

// LLMConfig 是模型服务配置。
type LLMConfig struct {
	Name     string `mapstructure:"name" yaml:"name"`
	Provider string `mapstructure:"provider" yaml:"provider"`
	APIKey   string `mapstructure:"api_key" yaml:"api_key"`
	BaseURL  string `mapstructure:"base_url" yaml:"base_url"`
	Model    string `mapstructure:"model" yaml:"model"`
}

// DatabaseConfig 是持久化存储配置。
type DatabaseConfig struct {
	MySQL MySQLConfig `mapstructure:"mysql" yaml:"mysql"`
	Redis RedisConfig `mapstructure:"redis" yaml:"redis"`
}

// MySQLConfig 是结构化的 MySQL 连接配置。
type MySQLConfig struct {
	Addr     string `mapstructure:"addr" yaml:"addr"`
	DBName   string `mapstructure:"dbname" yaml:"dbname"`
	Username string `mapstructure:"username" yaml:"username"`
	Password string `mapstructure:"password" yaml:"password"`
}

// RedisConfig 是短期记忆缓存配置。
type RedisConfig struct {
	Addr         string `mapstructure:"addr" yaml:"addr"`
	Password     string `mapstructure:"password" yaml:"password"`
	DB           int    `mapstructure:"db" yaml:"db"`
	ShortTermTTL string `mapstructure:"short_term_ttl" yaml:"short_term_ttl"`
}

// AgentConfig 是 Agent 运行配置。
type AgentConfig struct {
	ID                  string `mapstructure:"id" yaml:"id"`
	Name                string `mapstructure:"name" yaml:"name"`
	Description         string `mapstructure:"description" yaml:"description"`
	Instruction         string `mapstructure:"instruction" yaml:"instruction"`
	MaxHistoryMessages  int    `mapstructure:"max_history_messages" yaml:"max_history_messages"`
	MaxStreamChunkRunes int    `mapstructure:"max_stream_chunk_runes" yaml:"max_stream_chunk_runes"`
}

// Default 返回应用默认配置。敏感配置仍需要在 config.yaml 中显式填写。
func Default() Config {
	return Config{
		LLM: []LLMConfig{defaultLLMConfig()},
		Agent: AgentConfig{
			ID:                  "agent-1",
			Name:                "gora-eino-agent",
			Description:         "基于 Eino 的 Gora Agent",
			Instruction:         "你是一个智能助手，可以使用工具来完成任务。当需要获取外部信息时，请调用合适的工具。当你已经获得足够信息可以回答用户时，请直接给出回答。",
			MaxHistoryMessages:  30,
			MaxStreamChunkRunes: 64,
		},
	}
}

// Load 从 YAML 文件加载并校验应用配置。
func Load(path string) (Config, error) {
	cfg, err := Read(path)
	if err != nil {
		return Config{}, err
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Read 从 YAML 文件读取应用配置并补齐默认值，但不做 API Key 启动校验。
func Read(path string) (Config, error) {
	if strings.TrimSpace(path) == "" {
		path = DefaultPath
	}

	v := viper.New()
	v.SetConfigFile(path)

	if err := v.ReadInConfig(); err != nil {
		return Config{}, fmt.Errorf("读取配置文件 %s 失败: %w", path, err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return Config{}, fmt.Errorf("解析配置文件 %s 失败: %w", path, err)
	}

	cfg.applyDefaults()
	if err := cfg.ValidateRuntime(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Validate 校验当前配置是否足以启动应用。
func (c Config) Validate() error {
	return c.validate(true)
}

// ValidateRuntime 校验当前配置结构是否合法，但允许 API Key 在运行时补全。
func (c Config) ValidateRuntime() error {
	return c.validate(false)
}

func (c Config) validate(requireAPIKey bool) error {
	if len(c.LLM) == 0 {
		return fmt.Errorf("至少需要配置一个 llm")
	}

	seenNames := make(map[string]struct{}, len(c.LLM))
	for i, llmConfig := range c.LLM {
		if name := strings.ToLower(strings.TrimSpace(llmConfig.Name)); name != "" {
			if _, exists := seenNames[name]; exists {
				return fmt.Errorf("llm[%d].name 重复: %s", i, llmConfig.Name)
			}
			seenNames[name] = struct{}{}
		}

		if err := llmConfig.Validate(requireAPIKey); err != nil {
			return fmt.Errorf("llm[%d]: %w", i, err)
		}
	}

	if err := c.Database.Validate(); err != nil {
		return err
	}

	if err := c.RedisSettings().Validate(); err != nil {
		return err
	}

	return nil
}

// DatabaseURL 返回最终用于连接 MySQL 的 DSN。
func (c Config) DatabaseURL() string {
	return c.Database.ConnectionURL()
}

// RedisSettings 返回最终生效的 Redis 配置，优先使用新的 database.redis 结构。
func (c Config) RedisSettings() RedisConfig {
	if c.Database.Redis.HasSettings() {
		return c.Database.Redis
	}
	return c.Redis
}

// FindLLM 根据序号、name 或 model 查找目标 LLM。
func (c Config) FindLLM(selector string) (int, LLMConfig, error) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return 0, LLMConfig{}, fmt.Errorf("模型选择器不能为空")
	}

	if index, err := strconv.Atoi(selector); err == nil {
		if index < 1 || index > len(c.LLM) {
			return 0, LLMConfig{}, fmt.Errorf("模型序号超出范围: %d", index)
		}
		return index - 1, c.LLM[index-1], nil
	}

	for index, llmConfig := range c.LLM {
		if strings.EqualFold(strings.TrimSpace(llmConfig.Name), selector) {
			return index, llmConfig, nil
		}
	}

	for index, llmConfig := range c.LLM {
		if strings.EqualFold(strings.TrimSpace(llmConfig.Model), selector) {
			return index, llmConfig, nil
		}
	}

	return 0, LLMConfig{}, fmt.Errorf("未找到模型: %s", selector)
}

// DisplayName 返回用于展示和 `/model` 切换的可读名称。
func (c LLMConfig) DisplayName(index int) string {
	name := strings.TrimSpace(c.Name)
	model := strings.TrimSpace(c.Model)

	switch {
	case name != "" && model != "" && !strings.EqualFold(name, model):
		return fmt.Sprintf("%s (%s)", name, model)
	case name != "":
		return name
	case model != "":
		return model
	default:
		return fmt.Sprintf("llm-%d", index+1)
	}
}

// Validate 校验单个 LLM 配置。
func (c LLMConfig) Validate(requireAPIKey bool) error {
	provider := normalizeProvider(c.Provider)
	if provider != "deepseek" && provider != "openai" {
		return fmt.Errorf("provider 暂不支持: %s", c.Provider)
	}
	if requireAPIKey && strings.TrimSpace(c.APIKey) == "" {
		return fmt.Errorf("api_key 不能为空")
	}
	return nil
}

// ChatModelConfig 转换为 LLM 层使用的 OpenAI 兼容配置。
func (c LLMConfig) ChatModelConfig() llm.OpenAICompatibleConfig {
	return llm.OpenAICompatibleConfig{
		APIKey:  strings.TrimSpace(c.APIKey),
		BaseURL: strings.TrimSpace(c.BaseURL),
		Model:   strings.TrimSpace(c.Model),
	}
}

// DeepSeekConfig 保留给旧调用方。
func (c LLMConfig) DeepSeekConfig() llm.DeepSeekConfig {
	return c.ChatModelConfig()
}

func (c *Config) applyDefaults() {
	defaults := Default()
	defaultLLM := defaults.LLM[0]

	if len(c.LLM) == 0 {
		c.LLM = append(c.LLM, defaultLLM)
	}

	for i := range c.LLM {
		c.LLM[i].Name = strings.TrimSpace(c.LLM[i].Name)
		c.LLM[i].Provider = normalizeProvider(c.LLM[i].Provider)
		c.LLM[i].APIKey = strings.TrimSpace(c.LLM[i].APIKey)
		c.LLM[i].BaseURL = strings.TrimSpace(c.LLM[i].BaseURL)
		c.LLM[i].Model = strings.TrimSpace(c.LLM[i].Model)

		if c.LLM[i].Provider == "" {
			c.LLM[i].Provider = defaultLLM.Provider
		}

		providerDefaults := defaultLLMConfigForProvider(c.LLM[i].Provider)
		if c.LLM[i].BaseURL == "" {
			c.LLM[i].BaseURL = providerDefaults.BaseURL
		}
		if c.LLM[i].Model == "" {
			c.LLM[i].Model = providerDefaults.Model
		}
	}

	if strings.TrimSpace(c.Agent.ID) == "" {
		c.Agent.ID = defaults.Agent.ID
	}
	if strings.TrimSpace(c.Agent.Name) == "" {
		c.Agent.Name = defaults.Agent.Name
	}
	if strings.TrimSpace(c.Agent.Description) == "" {
		c.Agent.Description = defaults.Agent.Description
	}
	if strings.TrimSpace(c.Agent.Instruction) == "" {
		c.Agent.Instruction = defaults.Agent.Instruction
	}
	if c.Agent.MaxHistoryMessages <= 0 {
		c.Agent.MaxHistoryMessages = defaults.Agent.MaxHistoryMessages
	}
	if c.Agent.MaxStreamChunkRunes <= 0 {
		c.Agent.MaxStreamChunkRunes = defaults.Agent.MaxStreamChunkRunes
	}

	c.Database.MySQL.Addr = strings.TrimSpace(c.Database.MySQL.Addr)
	c.Database.MySQL.DBName = strings.TrimSpace(c.Database.MySQL.DBName)
	c.Database.MySQL.Username = strings.TrimSpace(c.Database.MySQL.Username)
	c.Database.MySQL.Password = strings.TrimSpace(c.Database.MySQL.Password)

	c.Database.Redis.Addr = strings.TrimSpace(c.Database.Redis.Addr)
	c.Database.Redis.Password = strings.TrimSpace(c.Database.Redis.Password)
	c.Database.Redis.ShortTermTTL = strings.TrimSpace(c.Database.Redis.ShortTermTTL)

	c.Redis.Addr = strings.TrimSpace(c.Redis.Addr)
	c.Redis.Password = strings.TrimSpace(c.Redis.Password)
	c.Redis.ShortTermTTL = strings.TrimSpace(c.Redis.ShortTermTTL)

	if !c.Database.Redis.HasSettings() && c.Redis.HasSettings() {
		c.Database.Redis = c.Redis
	}
	if c.Database.Redis.ShortTermTTL == "" {
		c.Database.Redis.ShortTermTTL = "24h"
	}
	c.Redis = c.Database.Redis
}

func defaultLLMConfig() LLMConfig {
	return defaultLLMConfigForProvider("deepseek")
}

func defaultLLMConfigForProvider(provider string) LLMConfig {
	switch normalizeProvider(provider) {
	case "openai":
		return LLMConfig{
			Provider: "openai",
			BaseURL:  "https://api.openai.com/v1",
			Model:    "gpt-4o-mini",
		}
	default:
		return LLMConfig{
			Provider: "deepseek",
			BaseURL:  "https://api.deepseek.com",
			Model:    "deepseek-chat",
		}
	}
}

// ConnectionURL 返回最终生效的 MySQL 连接串，优先使用兼容旧格式的 database.url。
func (c DatabaseConfig) ConnectionURL() string {
	return c.MySQL.DSN()
}

// Validate 校验数据库配置。
func (c DatabaseConfig) Validate() error {
	if err := c.MySQL.Validate(); err != nil {
		return fmt.Errorf("database.mysql: %w", err)
	}
	return nil
}

// HasSettings 判断 MySQL 结构化配置是否被设置。
func (c MySQLConfig) HasSettings() bool {
	return strings.TrimSpace(c.Addr) != "" || strings.TrimSpace(c.DBName) != "" || strings.TrimSpace(c.Username) != "" || strings.TrimSpace(c.Password) != ""
}

// Validate 校验结构化 MySQL 配置。
func (c MySQLConfig) Validate() error {
	if !c.HasSettings() {
		return nil
	}
	if strings.TrimSpace(c.Addr) == "" {
		return fmt.Errorf("addr 不能为空")
	}
	if strings.TrimSpace(c.DBName) == "" {
		return fmt.Errorf("dbname 不能为空")
	}
	if strings.TrimSpace(c.Username) == "" {
		return fmt.Errorf("username 不能为空")
	}
	return nil
}

// DSN 将结构化 MySQL 配置转换为 gorm/mysql 使用的连接串。
func (c MySQLConfig) DSN() string {
	if !c.HasSettings() {
		return ""
	}
	return fmt.Sprintf("%s:%s@tcp(%s)/%s?charset=utf8mb4&parseTime=True&loc=Local", c.Username, c.Password, c.Addr, c.DBName)
}

// HasSettings 判断 Redis 配置是否被显式设置。
func (c RedisConfig) HasSettings() bool {
	return strings.TrimSpace(c.Addr) != "" || strings.TrimSpace(c.Password) != "" || c.DB != 0 || strings.TrimSpace(c.ShortTermTTL) != ""
}

// Validate 校验 Redis 配置。
func (c RedisConfig) Validate() error {
	if c.DB < 0 {
		return fmt.Errorf("redis.db 不能为负数")
	}
	return nil
}

func normalizeProvider(provider string) string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" {
		return defaultLLMConfig().Provider
	}
	return provider
}
