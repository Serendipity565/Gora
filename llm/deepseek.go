package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DeepSeekConfig configures the DeepSeek OpenAI-compatible API client.
type DeepSeekConfig struct {
	APIKey  string
	BaseURL string
	Model   string
}

func DefaultDeepSeekConfig(apiKey string) DeepSeekConfig {
	return DeepSeekConfig{
		APIKey:  apiKey,
		BaseURL: "https://api.deepseek.com",
		Model:   "deepseek-chat",
	}
}

// DeepSeekLLM adapts DeepSeek's chat completions API to the LLM interface.
type DeepSeekLLM struct {
	config DeepSeekConfig
	client *http.Client
}

func NewDeepSeekLLM(config DeepSeekConfig) *DeepSeekLLM {
	if config.BaseURL == "" {
		config.BaseURL = "https://api.deepseek.com"
	}
	if config.Model == "" {
		config.Model = "deepseek-chat"
	}

	return &DeepSeekLLM{
		config: config,
		client: &http.Client{Timeout: 120 * time.Second},
	}
}

type chatRequest struct {
	Model    string           `json:"model"`
	Messages []Message        `json:"messages"`
	Tools    []map[string]any `json:"tools,omitempty"`
	Stream   bool             `json:"stream"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Role      string     `json:"role"`
			Content   string     `json:"content"`
			ToolCalls []ToolCall `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

func (d *DeepSeekLLM) Chat(ctx context.Context, messages []Message, tools []map[string]any) (*Message, error) {
	reqBody := chatRequest{
		Model:    d.config.Model,
		Messages: messages,
		Tools:    tools,
		Stream:   false,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.chatCompletionsURL(), bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+d.config.APIKey)

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API 错误 (%d): %s", resp.StatusCode, string(body))
	}

	var result chatResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("空响应")
	}

	choice := result.Choices[0]
	return &Message{
		Role:      Role(choice.Message.Role),
		Content:   choice.Message.Content,
		ToolCalls: choice.Message.ToolCalls,
	}, nil
}

func (d *DeepSeekLLM) ChatStream(ctx context.Context, messages []Message, tools []map[string]any) <-chan StreamChunk {
	ch := make(chan StreamChunk, 64)

	go func() {
		defer close(ch)

		reqBody := chatRequest{
			Model:    d.config.Model,
			Messages: messages,
			Tools:    tools,
			Stream:   true,
		}

		jsonBody, err := json.Marshal(reqBody)
		if err != nil {
			ch <- StreamChunk{Error: fmt.Errorf("序列化请求失败: %w", err)}
			return
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.chatCompletionsURL(), bytes.NewReader(jsonBody))
		if err != nil {
			ch <- StreamChunk{Error: fmt.Errorf("创建请求失败: %w", err)}
			return
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+d.config.APIKey)
		req.Header.Set("Accept", "text/event-stream")

		resp, err := d.client.Do(req)
		if err != nil {
			ch <- StreamChunk{Error: fmt.Errorf("请求失败: %w", err)}
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			ch <- StreamChunk{Error: fmt.Errorf("API 错误 (%d): %s", resp.StatusCode, string(body))}
			return
		}

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

		var accumulatedToolCalls []ToolCall

		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data:") {
				continue
			}

			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "" {
				continue
			}
			if data == "[DONE]" {
				ch <- StreamChunk{Done: true}
				return
			}

			var chunk struct {
				Choices []struct {
					Delta struct {
						Content   string     `json:"content"`
						ToolCalls []ToolCall `json:"tool_calls"`
					} `json:"delta"`
					FinishReason *string `json:"finish_reason"`
				} `json:"choices"`
			}

			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}

			if len(chunk.Choices) == 0 {
				continue
			}

			delta := chunk.Choices[0].Delta
			if len(delta.ToolCalls) > 0 {
				accumulatedToolCalls = mergeToolCalls(accumulatedToolCalls, delta.ToolCalls)
			}

			streamChunk := StreamChunk{
				Content:   delta.Content,
				ToolCalls: accumulatedToolCalls,
			}
			if chunk.Choices[0].FinishReason != nil {
				streamChunk.Done = true
			}

			ch <- streamChunk
		}

		if err := scanner.Err(); err != nil {
			ch <- StreamChunk{Error: fmt.Errorf("读取流失败: %w", err)}
		}
	}()

	return ch
}

func (d *DeepSeekLLM) chatCompletionsURL() string {
	return strings.TrimRight(d.config.BaseURL, "/") + "/v1/chat/completions"
}

func mergeToolCalls(existing, incoming []ToolCall) []ToolCall {
	for _, inc := range incoming {
		i := findToolCall(existing, inc)
		if i == -1 {
			existing = append(existing, inc)
			continue
		}

		if inc.Index != nil {
			existing[i].Index = inc.Index
		}
		if inc.ID != "" {
			existing[i].ID = inc.ID
		}
		if inc.Type != "" {
			existing[i].Type = inc.Type
		}
		existing[i].Function.Name += inc.Function.Name
		existing[i].Function.Arguments += inc.Function.Arguments
	}

	return existing
}

func findToolCall(existing []ToolCall, incoming ToolCall) int {
	if incoming.Index != nil {
		for i, ex := range existing {
			if ex.Index != nil && *ex.Index == *incoming.Index {
				return i
			}
		}
	}

	if incoming.ID != "" {
		for i, ex := range existing {
			if ex.ID == incoming.ID {
				return i
			}
		}
	}

	if incoming.Index == nil && incoming.ID == "" && len(existing) > 0 {
		return len(existing) - 1
	}

	return -1
}
