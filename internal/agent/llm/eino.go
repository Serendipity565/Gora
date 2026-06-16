// Package llm 封装大语言模型的创建与配置。
package llm

import (
	"context"
	"net/http"
	"strings"
	"time"

	einoopenai "github.com/cloudwego/eino-ext/components/model/openai"
	einomodel "github.com/cloudwego/eino/components/model"
)

// ChatModelConfig 是 Gora 使用的统一模型配置。
type ChatModelConfig struct {
	APIStyle string
	APIKey   string
	BaseURL  string
	Model    string
}

// OpenAICompatibleConfig 是 OpenAI 兼容接口的客户端配置。
type OpenAICompatibleConfig struct {
	APIKey  string // 接口密钥
	BaseURL string // 接口基础地址，例如 https://api.deepseek.com 或 https://api.openai.com/v1
	Model   string // 模型名称，例如 deepseek-chat 或 gpt-4o-mini
}

// DeepSeekConfig 保留给旧调用方，底层仍然使用 OpenAI 兼容配置。
type DeepSeekConfig = OpenAICompatibleConfig

// DefaultDeepSeekConfig 返回 DeepSeek 默认配置。
func DefaultDeepSeekConfig(apiKey string) OpenAICompatibleConfig {
	return OpenAICompatibleConfig{
		APIKey:  apiKey,
		BaseURL: "https://api.deepseek.com",
		Model:   "deepseek-chat",
	}
}

// NewEinoModel 根据配置创建 OpenAI 兼容的 Eino 模型。
func NewEinoModel(ctx context.Context, config ChatModelConfig) (einomodel.ToolCallingChatModel, error) {
	return NewOpenAICompatibleEinoModel(ctx, OpenAICompatibleConfig{
		APIKey:  config.APIKey,
		BaseURL: config.BaseURL,
		Model:   config.Model,
	})
}

// NewOpenAICompatibleEinoModel 创建一个基于 Eino OpenAI 兼容适配器的模型。
func NewOpenAICompatibleEinoModel(ctx context.Context, config OpenAICompatibleConfig) (*einoopenai.ChatModel, error) {
	return einoopenai.NewChatModel(ctx, &einoopenai.ChatModelConfig{
		APIKey:     strings.TrimSpace(config.APIKey),
		BaseURL:    strings.TrimRight(strings.TrimSpace(config.BaseURL), "/"),
		Model:      strings.TrimSpace(config.Model),
		HTTPClient: &http.Client{Timeout: 120 * time.Second},
	})
}

// NewDeepSeekEinoModel 创建一个基于 Eino OpenAI 兼容适配器的 DeepSeek 模型。
func NewDeepSeekEinoModel(ctx context.Context, config DeepSeekConfig) (*einoopenai.ChatModel, error) {
	if strings.TrimSpace(config.BaseURL) == "" {
		config.BaseURL = "https://api.deepseek.com"
	}
	if strings.TrimSpace(config.Model) == "" {
		config.Model = "deepseek-chat"
	}
	return NewOpenAICompatibleEinoModel(ctx, OpenAICompatibleConfig(config))
}
