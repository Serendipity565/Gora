package configs

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/viper"

	"github.com/Serendipity565/gora/internal/agent/llm"
)

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

	return cfg, nil
}

// Load 从 YAML 文件加载并校验应用配置，失败直接 panic。
//
// 仅供 cmd 里"启动失败应当 fail-fast"的场景使用；服务化代码请用 Read+Validate。
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

// Validate 校验当前配置是否足以启动应用。
func (c *Config) Validate() error {
	if len(c.LLM) == 0 {
		return errors.New("config.llm 至少需要配置一项")
	}
	if err := validateDuplicateLLMNames(c.LLM); err != nil {
		return err
	}
	for i, llmConfig := range c.LLM {
		if strings.TrimSpace(llmConfig.Name) == "" {
			return fmt.Errorf("config.llm[%d].name 不能为空", i)
		}
		if strings.TrimSpace(llmConfig.Model) == "" {
			return fmt.Errorf("config.llm[%d].model 不能为空", i)
		}
	}
	if strings.TrimSpace(c.Agent.ID) == "" {
		return errors.New("config.agent.id 不能为空")
	}
	return nil
}

// FindLLM 按 LLMConfig.Name 查找目标模型。
//
// 项目约定：name 是项目内模型的唯一标识；不再支持按序号 / model 字符串查找。
func (c *Config) FindLLM(name string) (int, LLMConfig, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, LLMConfig{}, fmt.Errorf("模型 name 不能为空")
	}
	for index, llmConfig := range c.LLM {
		if strings.EqualFold(strings.TrimSpace(llmConfig.Name), name) {
			return index, llmConfig, nil
		}
	}
	return 0, LLMConfig{}, fmt.Errorf("未找到模型: %s", name)
}

// DisplayName 返回展示用名称（与 Name 等价；index 仅作兜底）。
func (c *LLMConfig) DisplayName(index int) string {
	if name := strings.TrimSpace(c.Name); name != "" {
		return name
	}
	return fmt.Sprintf("llm-%d", index+1)
}

// ChatModelConfig 转换为 LLM 层使用的模型配置。
func (c *LLMConfig) ChatModelConfig() llm.ChatModelConfig {
	return llm.ChatModelConfig{
		APIStyle: normalizeAPIStyle(c.APIStyle),
		APIKey:   strings.TrimSpace(c.APIKey),
		BaseURL:  strings.TrimSpace(c.BaseURL),
		Model:    strings.TrimSpace(c.Model),
	}
}

// HasSettings 判断 MySQL 结构化配置是否被设置。
func (c *MySQLConfig) HasSettings() bool {
	return strings.TrimSpace(c.Addr) != "" ||
		strings.TrimSpace(c.DBName) != "" ||
		strings.TrimSpace(c.Username) != "" ||
		strings.TrimSpace(c.Password) != ""
}

// HasSettings 判断 Redis 配置是否被显式设置。
func (c *RedisConfig) HasSettings() bool {
	return strings.TrimSpace(c.Addr) != "" ||
		strings.TrimSpace(c.Password) != "" ||
		c.DB != 0
}

func normalizeAPIStyle(style string) string {
	return strings.ToLower(strings.TrimSpace(style))
}

func validateDuplicateLLMNames(configs []LLMConfig) error {
	seen := make(map[string]struct{}, len(configs))
	for i, llmConfig := range configs {
		name := strings.ToLower(strings.TrimSpace(llmConfig.Name))
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			return fmt.Errorf("llm[%d].name 重复: %s", i, llmConfig.Name)
		}
		seen[name] = struct{}{}
	}
	return nil
}
