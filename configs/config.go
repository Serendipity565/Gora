// Package configs 定义应用配置结构与 yaml 加载逻辑。
package configs

import (
	"github.com/google/wire"
)

// DefaultPath 是 Gora CLI 默认读取的配置文件路径。
const DefaultPath = "configs/config.yaml"

// ProviderSet 把 Config 拆成各子配置，便于 wire 在不重复读 yaml 的前提下注入。
//
// 注意：不包含 NewConfig —— 应用入口（cmd/gora/main.go）一般会自己读 yaml 后把
// Config 作为入参传给 wire 注入器，避免 wire 多次读盘。如果调用方只想要"读 yaml +
// 拆子配置"的全套，请用 ProviderSetWithLoader。
var ProviderSet = wire.NewSet(
	NewServerConfig,
	NewLLMConfigs,
	NewAgentConfig,
	NewMysqlConfig,
	NewRedisConfig,
	NewJWTConfig,
	NewBasicAuthAccounts,
	NewLimiterConfig,
	NewLogConfig,
	NewCorsConfig,
	NewAdminConfig,
)

// ProviderSetWithLoader 在 ProviderSet 之上额外暴露 NewConfig，用于完全把
// 配置加载交给 wire 的场景（例如脚本 / 测试）。
var ProviderSetWithLoader = wire.NewSet(
	NewConfig,
	ProviderSet,
)

// Config 是应用启动配置。
type Config struct {
	Server     ServerConfig     `mapstructure:"server" yaml:"server"`
	LLM        []LLMConfig      `mapstructure:"llm" yaml:"llm"`
	Agent      AgentConfig      `mapstructure:"agent" yaml:"agent"`
	Database   DatabaseConfig   `mapstructure:"database" yaml:"database"`
	Middleware MiddlewareConfig `mapstructure:"middleware" yaml:"middleware"`
	Admin      AdminConfig      `mapstructure:"admin" yaml:"admin"`
}

// ServerConfig 控制 HTTP 服务器自身的运行时参数。
type ServerConfig struct {
	// Addr HTTP 监听地址，例如 ":8080"；空时默认 ":8080"。
	Addr string `mapstructure:"addr" yaml:"addr"`
	// CORS 是否启用 CORS 中间件（前端独立 dev server 跨域时用）。
	CORS bool `mapstructure:"cors" yaml:"cors"`
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

// LogConfig 控制访问日志中间件 + zap 文件轮转。
//
// 字段拆成两类：
//   - Level / SkipPaths：访问日志中间件用；
//   - File / MaxSize / MaxBackups / MaxAge / Compress：底层 zap + lumberjack 切割用，
//     与 ioc.InitLogger 一一对应。
type LogConfig struct {
	// Level 取值 debug / info / warn / error；空字符串视为 info。
	Level string `mapstructure:"level" yaml:"level"`
	// SkipPaths 列出不写访问日志的路径前缀（精确匹配）。
	SkipPaths []string `mapstructure:"skip_paths" yaml:"skip_paths"`

	// File 日志文件路径；空时 lumberjack 会落到默认 ./<bin>.log。
	File string `mapstructure:"file" yaml:"file"`
	// MaxSize 单个文件触发切割的大小（MB）。
	MaxSize int `mapstructure:"max_size" yaml:"max_size"`
	// MaxBackups 保留旧文件的最大个数。
	MaxBackups int `mapstructure:"max_backups" yaml:"max_backups"`
	// MaxAge 保留旧文件的最大天数。
	MaxAge int `mapstructure:"max_age" yaml:"max_age"`
	// Compress 是否压缩旧文件。
	Compress bool `mapstructure:"compress" yaml:"compress"`
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
	APIStyle string `mapstructure:"api_style" yaml:"api_style"`
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

// AdminConfig 是启动期 seed 的内置管理员账号；所有字段非空时该账号才会被插入。
//
// 启动顺序：
//  1. AutoMigrate user 表（dao.NewUserDAO 会做）；
//  2. 按 Email 查找；
//  3. 不存在则用 bcrypt 加密 Password 后插入；存在则跳过（不更新密码）。
type AdminConfig struct {
	Email    string `mapstructure:"email" yaml:"email"`
	Password string `mapstructure:"password" yaml:"password"`
	Username string `mapstructure:"username" yaml:"username"`
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

// NewConfig 从默认路径加载配置，校验失败直接 panic。
//
// 仅用于"完全交给 wire 装配"的场景；正常应用入口请走 Read + Validate，自行处理错误。
func NewConfig() Config {
	return Load(DefaultPath)
}

func NewLLMConfigs(cfg Config) []LLMConfig {
	return cfg.LLM
}

func NewServerConfig(cfg Config) ServerConfig {
	return cfg.Server
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

func NewAdminConfig(cfg Config) AdminConfig {
	return cfg.Admin
}
