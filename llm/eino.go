package llm

import (
	"context"
	"net/http"
	"strings"
	"time"

	einoopenai "github.com/cloudwego/eino-ext/components/model/openai"
)

// NewDeepSeekEinoModel 创建一个基于 Eino OpenAI 兼容适配器的 DeepSeek 模型。
func NewDeepSeekEinoModel(ctx context.Context, config DeepSeekConfig) (*einoopenai.ChatModel, error) {
	if config.BaseURL == "" {
		config.BaseURL = "https://api.deepseek.com"
	}
	if config.Model == "" {
		config.Model = "deepseek-chat"
	}

	return einoopenai.NewChatModel(ctx, &einoopenai.ChatModelConfig{
		APIKey:     config.APIKey,
		BaseURL:    strings.TrimRight(config.BaseURL, "/"),
		Model:      config.Model,
		HTTPClient: &http.Client{Timeout: 120 * time.Second},
	})
}
