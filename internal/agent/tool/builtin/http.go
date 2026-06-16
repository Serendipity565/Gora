// Package builtin 提供内置工具实现，如 HTTP 请求工具。
package builtin

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

type HTTPToolOption func(*HTTPTool)

// HTTPTool 是内置 HTTP 请求工具。
type HTTPTool struct {
	client               *http.Client
	allowedMethods       map[string]struct{}
	allowPrivateNetworks bool
}

// NewHTTPTool 创建一个带默认超时时间和安全边界的 HTTP 工具。
func NewHTTPTool(opts ...HTTPToolOption) *HTTPTool {
	h := &HTTPTool{
		allowedMethods: map[string]struct{}{
			http.MethodGet: {},
		},
	}

	for _, opt := range opts {
		opt(h)
	}

	h.client = &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("重定向次数过多")
			}
			_, err := h.validateURL(req.Context(), req.URL.String())
			return err
		},
	}

	return h
}

func WithAllowedMethods(methods ...string) HTTPToolOption {
	return func(h *HTTPTool) {
		allowed := make(map[string]struct{}, len(methods))
		for _, method := range methods {
			method = strings.ToUpper(strings.TrimSpace(method))
			if method != "" {
				allowed[method] = struct{}{}
			}
		}
		if len(allowed) > 0 {
			h.allowedMethods = allowed
		}
	}
}

func WithPrivateNetworkAccess(allow bool) HTTPToolOption {
	return func(h *HTTPTool) {
		h.allowPrivateNetworks = allow
	}
}

// Name 返回工具名称。
func (h *HTTPTool) Name() string {
	return "http_request"
}

// Description 返回工具说明，供 LLM 选择工具时参考。
func (h *HTTPTool) Description() string {
	return "发送安全受限的 HTTP 请求到公开 URL，默认仅支持 GET"
}

// Parameters 返回工具参数的 JSON 模式定义。
func (h *HTTPTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url": map[string]any{
				"type":        "string",
				"description": "请求的公开 HTTP/HTTPS URL",
			},
			"method": map[string]any{
				"type":        "string",
				"description": "HTTP 方法，默认 GET",
				"enum":        h.allowedMethodList(),
			},
			"body": map[string]any{
				"type":        "string",
				"description": "请求体（JSON 字符串，仅在允许非 GET 方法时使用）",
			},
		},
		"required": []string{"url"},
	}
}

// Execute 按参数发起 HTTP 请求，并返回状态码和响应体摘要。
func (h *HTTPTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	rawURL, ok := args["url"].(string)
	if !ok || strings.TrimSpace(rawURL) == "" {
		return "", fmt.Errorf("缺少 url 参数")
	}

	targetURL, err := h.validateURL(ctx, rawURL)
	if err != nil {
		return "", err
	}

	method := http.MethodGet
	if m, ok := args["method"].(string); ok && m != "" {
		method = strings.ToUpper(strings.TrimSpace(m))
	}
	if !h.isMethodAllowed(method) {
		return "", fmt.Errorf("HTTP 方法 %s 未被允许", method)
	}

	var body io.Reader
	if b, ok := args["body"].(string); ok && b != "" {
		body = strings.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, targetURL.String(), body)
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

func (h *HTTPTool) validateURL(ctx context.Context, rawURL string) (*url.URL, error) {
	targetURL, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil, fmt.Errorf("解析 URL 失败: %w", err)
	}

	switch strings.ToLower(targetURL.Scheme) {
	case "http", "https":
	default:
		return nil, fmt.Errorf("仅支持 http 和 https URL")
	}

	host := targetURL.Hostname()
	if host == "" {
		return nil, fmt.Errorf("URL 缺少主机名")
	}

	if !h.allowPrivateNetworks {
		if err := rejectPrivateHost(ctx, host); err != nil {
			return nil, err
		}
	}

	return targetURL, nil
}

func (h *HTTPTool) isMethodAllowed(method string) bool {
	_, ok := h.allowedMethods[method]
	return ok
}

func (h *HTTPTool) allowedMethodList() []string {
	methods := make([]string, 0, len(h.allowedMethods))
	for method := range h.allowedMethods {
		methods = append(methods, method)
	}
	sort.Strings(methods)
	return methods
}

func rejectPrivateHost(ctx context.Context, host string) error {
	if ip := net.ParseIP(host); ip != nil {
		if isBlockedIP(ip) {
			return fmt.Errorf("禁止访问私有或本地地址: %s", ip.String())
		}
		return nil
	}

	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return fmt.Errorf("解析主机失败: %w", err)
	}

	for _, addr := range addrs {
		if isBlockedIP(addr.IP) {
			return fmt.Errorf("禁止访问私有或本地地址: %s (%s)", host, addr.IP.String())
		}
	}

	return nil
}

func isBlockedIP(ip net.IP) bool {
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsMulticast()
}
