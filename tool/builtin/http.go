package builtin

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// HTTPTool 是内置 HTTP 请求工具。
type HTTPTool struct {
	client *http.Client
}

// NewHTTPTool 创建一个带默认超时时间的 HTTP 工具。
func NewHTTPTool() *HTTPTool {
	return &HTTPTool{
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// Name 返回工具名称。
func (h *HTTPTool) Name() string {
	return "http_request"
}

// Description 返回工具说明，供 LLM 选择工具时参考。
func (h *HTTPTool) Description() string {
	return "发送 HTTP 请求到指定 URL，支持 GET/POST 等方法"
}

// Parameters 返回工具参数的 JSON 模式定义。
func (h *HTTPTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url": map[string]any{
				"type":        "string",
				"description": "请求的 URL",
			},
			"method": map[string]any{
				"type":        "string",
				"description": "HTTP 方法，默认 GET",
				"enum":        []string{"GET", "POST", "PUT", "DELETE"},
			},
			"body": map[string]any{
				"type":        "string",
				"description": "请求体（JSON 字符串）",
			},
		},
		"required": []string{"url"},
	}
}

// Execute 按参数发起 HTTP 请求，并返回状态码和响应体摘要。
func (h *HTTPTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	url, ok := args["url"].(string)
	if !ok || url == "" {
		return "", fmt.Errorf("缺少 url 参数")
	}

	method := "GET"
	if m, ok := args["method"].(string); ok && m != "" {
		method = strings.ToUpper(m)
	}

	var body io.Reader
	if b, ok := args["body"].(string); ok && b != "" {
		body = strings.NewReader(b)
	}

	// 请求绑定 ctx，方便上层取消 Agent 运行时中断网络调用。
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 限制响应大小，避免工具结果过大影响后续 LLM 上下文。
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %w", err)
	}

	return fmt.Sprintf("HTTP %d\n%s", resp.StatusCode, string(data)), nil
}
