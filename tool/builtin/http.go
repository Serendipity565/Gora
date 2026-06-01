package builtin

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type HTTPTool struct {
	client *http.Client
}

func NewHTTPTool() *HTTPTool {
	return &HTTPTool{
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (h *HTTPTool) Name() string {
	return "http_request"
}

func (h *HTTPTool) Description() string {
	return "发送 HTTP 请求到指定 URL，支持 GET/POST 等方法"
}

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

	data, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %w", err)
	}

	return fmt.Sprintf("HTTP %d\n%s", resp.StatusCode, string(data)), nil
}
