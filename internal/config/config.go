package config

import (
	"fmt"

	"github.com/google/wire"
	"github.com/spf13/viper"
)

// DefaultPath 是 Gora CLI 默认读取的配置文件路径。
const DefaultPath = "configs/config.yaml"

var ProviderSet = wire.NewSet(
	NewConfig,
	NewLLMConfigs,
	NewAgentConfig,
	NewMysqlConfig,
	NewRedisConfig,
	NewJWTConfig,
	NewBasicAuthAccounts,
	NewLimiterConfig,
	NewLogConfig,
	NewCorsConfig,
)

// Config 是应用启动配置。
type Config struct {
	LLM        []LLMConfig      `mapstructure:"llm" yaml:"llm"`
	Agent      AgentConfig      `mapstructure:"agent" yaml:"agent"`
	Database   DatabaseConfig   `mapstructure:"database" yaml:"database"`
	Middleware MiddlewareConfig `mapstructure:"middleware" yaml:"middleware"`
}

// MiddlewareConfig 聚合所有 Gin 中间件的可调参数。
type MiddlewareConfig struct {
	JWT       JWTConfig          `mapstructure:"jwt" yaml:"jwt"`
	BasicAuth []BasicAuthAccount `mapstructure:"basic_auth" yaml:"basic_auth"`
	Limiter   LimiterConfig      `mapstructure:"limiter" yaml:"limiter"`
	Log       LogConfig          `mapstructure:"log" yaml:"log"`
	Cors      CorsConfig         `mapstructure:"cors" yaml:"cors"`
}

// JWTConfig 是 JWT 鉴权中间件依赖的密钥与签发参数。
type JWTConfig struct {
	Secret string `mapstructure:"secret" yaml:"secret"`
	EncKey string `mapstructure:"enc_key" yaml:"enc_key"`
	// Expire 接受 time.ParseDuration 字符串（秒）；空值默认 24h。
	Expire string `mapstructure:"expire" yaml:"expire"`
}

// BasicAuthAccount 是 HTTP Basic Auth 中间件的单条凭据。
type BasicAuthAccount struct {
	Username string `mapstructure:"username" yaml:"username"`
	Password string `mapstructure:"password" yaml:"password"`
}

// LimiterConfig 是基于 Redis 令牌桶的限流参数。
type LimiterConfig struct {
	Capacity     int `mapstructure:"capacity" yaml:"capacity"`
	FillInterval int `mapstructure:"fill_interval" yaml:"fill_interval"`
	Quantum      int `mapstructure:"quantum" yaml:"quantum"`
}

// LogConfig 控制访问日志中间件。
type LogConfig struct {
	// Level 取值 debug / info / warn / error；空字符串视为 info。
	Level string `mapstructure:"level" yaml:"level"`
	// SkipPaths 列出不写访问日志的路径前缀（精确匹配）。
	SkipPaths []string `mapstructure:"skip_paths" yaml:"skip_paths"`
}

// CorsConfig 是跨域资源共享中间件配置。
type CorsConfig struct {
	AllowedOrigins []string `mapstructure:"allowed_origins" yaml:"allowed_origins"`
	AllowedMethods []string `mapstructure:"allowed_methods" yaml:"allowed_methods"`
	AllowedHeaders []string `mapstructure:"allowed_headers" yaml:"allowed_headers"`
}

// LLMConfig 是模型服务配置。
//
// 约定：
//   - Name 是项目内的"模型唯一标识"，所有查找 / 展示 / 持久化都用它。
//     必填、配置内不重复（大小写不敏感）。
//   - Model 仅在调用底层 LLM API 时作为请求参数（即 ChatModelConfig），
//     不参与任何 selector / 展示逻辑。
type LLMConfig struct {
	Name     string `mapstructure:"name" yaml:"name"`
	Provider string `mapstructure:"provider" yaml:"provider"`
	APIKey   string `mapstructure:"api_key" yaml:"api_key"`
	BaseURL  string `mapstructure:"base_url" yaml:"base_url"`
	Model    string `mapstructure:"model" yaml:"model"`
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

	MaxOpenConns    int    `mapstructure:"max_open_conns" yaml:"max_open_conns"`
	MaxIdleConns    int    `mapstructure:"max_idle_conns" yaml:"max_idle_conns"`
	ConnMaxLifetime string `mapstructure:"conn_max_lifetime" yaml:"conn_max_lifetime"`
}

// RedisConfig 是短期记忆缓存配置。
type RedisConfig struct {
	Addr     string `mapstructure:"addr" yaml:"addr"`
	Password string `mapstructure:"password" yaml:"password"`
	DB       int    `mapstructure:"db" yaml:"db"`
}

func NewConfig() Config {
	path := DefaultPath
	v := viper.New()
	v.SetConfigFile(path)

	if err := v.ReadInConfig(); err != nil {
		panic(fmt.Errorf("读取配置文件 %s 失败: %w", path, err))
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		panic(fmt.Errorf("解析配置文件 %s 失败: %w", path, err))
	}

	return cfg
}

func NewLLMConfigs(cfg Config) []LLMConfig {
	return cfg.LLM
}

func NewAgentConfig(cfg Config) AgentConfig {
	return cfg.Agent
}

func NewMysqlConfig(cfg Config) MySQLConfig {
	return cfg.Database.MySQL
}

func NewRedisConfig(cfg Config) RedisConfig {
	return cfg.Database.Redis
}

func NewJWTConfig(cfg Config) JWTConfig {
	return cfg.Middleware.JWT
}

func NewBasicAuthAccounts(cfg Config) []BasicAuthAccount {
	return cfg.Middleware.BasicAuth
}

func NewLimiterConfig(cfg Config) LimiterConfig {
	return cfg.Middleware.Limiter
}

func NewLogConfig(cfg Config) LogConfig {
	return cfg.Middleware.Log
}

func NewCorsConfig(cfg Config) CorsConfig {
	return cfg.Middleware.Cors
}
