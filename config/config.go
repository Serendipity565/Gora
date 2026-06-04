package config

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Serendipity565/gora/llm"
	"github.com/go-playground/validator/v10"
	"github.com/spf13/viper"
)

// DefaultPath 是 Gora CLI 默认读取的配置文件路径。
const DefaultPath = "config/config.yaml"

var validationNamespaceReplacer = strings.NewReplacer(
	"Config.", "",
	"DatabaseConfig.", "",
	"AgentConfig.", "",
	"RedisConfig.", "",
	"MySQLConfig.", "",
	"LLMConfig.", "",
	"MaxHistoryMessages", "max_history_messages",
	"MaxStreamChunkRunes", "max_stream_chunk_runes",
	"ShortTermTTL", "short_term_ttl",
	"BaseURL", "base_url",
	"DBName", "dbname",
	"APIKey", "api_key",
	"Database", "database",
	"Agent", "agent",
	"MySQL", "mysql",
	"Redis", "redis",
	"LLM", "llm",
	"Provider", "provider",
	"Model", "model",
	"Description", "description",
	"Instruction", "instruction",
	"Name", "name",
	"URL", "url",
	"Addr", "addr",
	"Username", "username",
	"Password", "password",
	"ID", "id",
	"DB", "db",
)

// Config 是应用启动配置。
type Config struct {
	LLM      []LLMConfig    `mapstructure:"llm" yaml:"llm" validate:"required,min=1,dive"`
	Agent    AgentConfig    `mapstructure:"agent" yaml:"agent"`
	Database DatabaseConfig `mapstructure:"database" yaml:"database"`
	Redis    RedisConfig    `mapstructure:"redis" yaml:"redis"`
}

// LLMConfig 是模型服务配置。
type LLMConfig struct {
	Name     string `mapstructure:"name" yaml:"name"`
	Provider string `mapstructure:"provider" yaml:"provider" validate:"required"`
	APIKey   string `mapstructure:"api_key" yaml:"api_key"`
	BaseURL  string `mapstructure:"base_url" yaml:"base_url"`
	Model    string `mapstructure:"model" yaml:"model" validate:"required"`
}

// DatabaseConfig 是持久化存储配置。
type DatabaseConfig struct {
	URL   string      `mapstructure:"url" yaml:"url"`
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
	DB           int    `mapstructure:"db" yaml:"db" validate:"gte=0"`
	ShortTermTTL string `mapstructure:"short_term_ttl" yaml:"short_term_ttl"`
}

// AgentConfig 是 Agent 运行配置。
type AgentConfig struct {
	ID                  string `mapstructure:"id" yaml:"id" validate:"required"`
	Name                string `mapstructure:"name" yaml:"name" validate:"required"`
	Description         string `mapstructure:"description" yaml:"description" validate:"required"`
	Instruction         string `mapstructure:"instruction" yaml:"instruction" validate:"required"`
	MaxHistoryMessages  int    `mapstructure:"max_history_messages" yaml:"max_history_messages" validate:"gte=1"`
	MaxStreamChunkRunes int    `mapstructure:"max_stream_chunk_runes" yaml:"max_stream_chunk_runes" validate:"gte=1"`
}

// Load 从 YAML 文件加载并校验应用配置，失败直接 panic。
func Load(path string) Config {
	cfg, err := Read(path)
	if err != nil {
		panic(err)
	}
	if err := cfg.Validate(); err != nil {
		panic(err)
	}
	return cfg
}

// Read 从 YAML 文件读取应用配置并做基础归一化。
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

	cfg.normalize()
	return cfg, nil
}

// Validate 校验当前配置是否足以启动应用。
func (c Config) Validate() error {
	if err := validateDuplicateLLMNames(c.LLM); err != nil {
		return err
	}

	v := newValidator()
	if err := v.Struct(c); err != nil {
		return formatValidationError(err)
	}

	return nil
}

func (c *Config) normalize() {
	for i := range c.LLM {
		c.LLM[i].Name = strings.TrimSpace(c.LLM[i].Name)
		c.LLM[i].Provider = normalizeProvider(c.LLM[i].Provider)
		c.LLM[i].APIKey = strings.TrimSpace(c.LLM[i].APIKey)
		c.LLM[i].BaseURL = strings.TrimSpace(c.LLM[i].BaseURL)
		c.LLM[i].Model = strings.TrimSpace(c.LLM[i].Model)
	}

	c.Agent.ID = strings.TrimSpace(c.Agent.ID)
	c.Agent.Name = strings.TrimSpace(c.Agent.Name)
	c.Agent.Description = strings.TrimSpace(c.Agent.Description)
	c.Agent.Instruction = strings.TrimSpace(c.Agent.Instruction)

	c.Database.URL = strings.TrimSpace(c.Database.URL)
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
	c.Redis = c.Database.Redis
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

// ChatModelConfig 转换为 LLM 层使用的模型配置。
func (c LLMConfig) ChatModelConfig() llm.ChatModelConfig {
	return llm.ChatModelConfig{
		Provider: normalizeProvider(c.Provider),
		APIKey:   strings.TrimSpace(c.APIKey),
		BaseURL:  strings.TrimSpace(c.BaseURL),
		Model:    strings.TrimSpace(c.Model),
	}
}

// DSN 返回最终生效的 MySQL 连接串。
func (c DatabaseConfig) DSN() string {
	if url := strings.TrimSpace(c.URL); url != "" {
		return url
	}
	if !c.MySQL.HasSettings() {
		return ""
	}
	return fmt.Sprintf(
		"%s:%s@tcp(%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		strings.TrimSpace(c.MySQL.Username),
		strings.TrimSpace(c.MySQL.Password),
		strings.TrimSpace(c.MySQL.Addr),
		strings.TrimSpace(c.MySQL.DBName),
	)
}

// HasSettings 判断 MySQL 结构化配置是否被设置。
func (c MySQLConfig) HasSettings() bool {
	return strings.TrimSpace(c.Addr) != "" || strings.TrimSpace(c.DBName) != "" || strings.TrimSpace(c.Username) != "" || strings.TrimSpace(c.Password) != ""
}

// HasSettings 判断 Redis 配置是否被显式设置。
func (c RedisConfig) HasSettings() bool {
	return strings.TrimSpace(c.Addr) != "" || strings.TrimSpace(c.Password) != "" || c.DB != 0 || strings.TrimSpace(c.ShortTermTTL) != ""
}

func normalizeProvider(provider string) string {
	return strings.ToLower(strings.TrimSpace(provider))
}

func validateDuplicateLLMNames(configs []LLMConfig) error {
	seenNames := make(map[string]struct{}, len(configs))
	for i, llmConfig := range configs {
		name := strings.ToLower(strings.TrimSpace(llmConfig.Name))
		if name == "" {
			continue
		}
		if _, exists := seenNames[name]; exists {
			return fmt.Errorf("llm[%d].name 重复: %s", i, llmConfig.Name)
		}
		seenNames[name] = struct{}{}
	}
	return nil
}

func newValidator() *validator.Validate {
	v := validator.New(validator.WithRequiredStructEnabled())
	v.RegisterStructValidation(validateLLMConfig, LLMConfig{})
	v.RegisterStructValidation(validateMySQLConfig, MySQLConfig{})
	v.RegisterStructValidation(validateRedisConfig, RedisConfig{})
	return v
}

func validateLLMConfig(sl validator.StructLevel) {
	config, ok := sl.Current().Interface().(LLMConfig)
	if !ok {
		return
	}

	provider := normalizeProvider(config.Provider)
	if provider == "" || provider == "mock" {
		return
	}

	if strings.TrimSpace(config.BaseURL) == "" {
		sl.ReportError(config.BaseURL, "base_url", "BaseURL", "required", "")
	}
	if strings.TrimSpace(config.APIKey) == "" {
		sl.ReportError(config.APIKey, "api_key", "APIKey", "required", "")
	}
}

func validateMySQLConfig(sl validator.StructLevel) {
	config, ok := sl.Current().Interface().(MySQLConfig)
	if !ok || !config.HasSettings() {
		return
	}

	if strings.TrimSpace(config.Addr) == "" {
		sl.ReportError(config.Addr, "addr", "Addr", "required", "")
	}
	if strings.TrimSpace(config.DBName) == "" {
		sl.ReportError(config.DBName, "dbname", "DBName", "required", "")
	}
	if strings.TrimSpace(config.Username) == "" {
		sl.ReportError(config.Username, "username", "Username", "required", "")
	}
}

func validateRedisConfig(sl validator.StructLevel) {
	config, ok := sl.Current().Interface().(RedisConfig)
	if !ok {
		return
	}

	if ttl := strings.TrimSpace(config.ShortTermTTL); ttl != "" {
		if _, err := time.ParseDuration(ttl); err != nil {
			sl.ReportError(config.ShortTermTTL, "short_term_ttl", "ShortTermTTL", "duration", "")
		}
	}
}

func formatValidationError(err error) error {
	var validationErrors validator.ValidationErrors
	if !errors.As(err, &validationErrors) {
		return err
	}

	messages := make([]string, 0, len(validationErrors))
	for _, validationError := range validationErrors {
		messages = append(messages, describeValidationError(validationError))
	}

	return errors.New(strings.Join(messages, "; "))
}

func describeValidationError(err validator.FieldError) string {
	field := validationFieldName(err)

	switch err.Tag() {
	case "required":
		return fmt.Sprintf("%s 不能为空", field)
	case "oneof":
		return fmt.Sprintf("%s 仅支持: %s", field, strings.ReplaceAll(err.Param(), " ", ", "))
	case "gte":
		return fmt.Sprintf("%s 必须大于等于 %s", field, err.Param())
	case "min":
		return fmt.Sprintf("%s 至少需要 %s 项", field, err.Param())
	case "duration":
		return fmt.Sprintf("%s 不是有效的 time.Duration", field)
	default:
		return fmt.Sprintf("%s 校验失败: %s", field, err.Tag())
	}
}

func validationFieldName(err validator.FieldError) string {
	field := strings.TrimSpace(err.Namespace())
	if field == "" {
		field = strings.TrimSpace(err.StructNamespace())
	}
	if field == "" {
		field = strings.TrimSpace(err.Field())
	}
	if field == "" {
		field = strings.TrimSpace(err.StructField())
	}
	field = validationNamespaceReplacer.Replace(field)
	field = strings.TrimPrefix(field, ".")
	return field
}
