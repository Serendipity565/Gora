package builtin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

// WebSearchTool 通过 Serper.dev（首选）和 DuckDuckGo HTML 端点（兜底）执行网页搜索。
//
// 选择策略由参数 engine 控制：
//   - "auto"（默认）   先尝试 Serper（需 SERPER_API_KEY），失败或缺 key 时降级 DuckDuckGo；
//   - "serper"         强制走 Serper，缺 key 时直接报错；
//   - "duckduckgo"     强制走 DuckDuckGo HTML，无需 API key。
//
// 工具不写日志、不缓存；请求超时 15s，最大返回结果数 10。
type WebSearchTool struct {
	client      *http.Client
	apiKey      string // Serper.dev API key；空则在 auto 模式下直接走 DuckDuckGo
	serperURL   string // 默认 https://google.serper.dev/search，测试可覆盖
	duckduckURL string // 默认 https://duckduckgo.com/html/，测试可覆盖
}

// WebSearchOption 用于自定义 WebSearchTool。
type WebSearchOption func(*WebSearchTool)

// WithWebSearchAPIKey 显式注入 Serper.dev API Key（覆盖环境变量 SERPER_API_KEY）。
func WithWebSearchAPIKey(key string) WebSearchOption {
	return func(w *WebSearchTool) {
		w.apiKey = strings.TrimSpace(key)
	}
}

// WithSerperEndpoint 覆盖 Serper.dev 接口地址，仅供测试使用。
func WithSerperEndpoint(endpoint string) WebSearchOption {
	return func(w *WebSearchTool) {
		if endpoint != "" {
			w.serperURL = endpoint
		}
	}
}

// WithDuckDuckGoEndpoint 覆盖 DuckDuckGo HTML 端点，仅供测试使用。
func WithDuckDuckGoEndpoint(endpoint string) WebSearchOption {
	return func(w *WebSearchTool) {
		if endpoint != "" {
			w.duckduckURL = endpoint
		}
	}
}

// WithWebSearchHTTPClient 覆盖底层 http.Client（默认带 15s 超时）。
func WithWebSearchHTTPClient(client *http.Client) WebSearchOption {
	return func(w *WebSearchTool) {
		if client != nil {
			w.client = client
		}
	}
}

// NewWebSearchTool 创建一个网页搜索工具。SERPER_API_KEY 环境变量存在时自动用作 Serper key。
func NewWebSearchTool(opts ...WebSearchOption) *WebSearchTool {
	w := &WebSearchTool{
		client:      &http.Client{Timeout: 15 * time.Second},
		apiKey:      strings.TrimSpace(os.Getenv("SERPER_API_KEY")),
		serperURL:   "https://google.serper.dev/search",
		duckduckURL: "https://duckduckgo.com/html/",
	}
	for _, opt := range opts {
		opt(w)
	}
	return w
}

func (w *WebSearchTool) Name() string {
	return "web_search"
}

func (w *WebSearchTool) Description() string {
	return "通过 Serper.dev（Google 搜索 API）或 DuckDuckGo 进行网页搜索，返回标题、链接和摘要。"
}

func (w *WebSearchTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "搜索关键词，必填",
			},
			"num_results": map[string]any{
				"type":        "integer",
				"description": "返回结果条数，默认 5，最大 10",
				"minimum":     1,
				"maximum":     10,
			},
			"engine": map[string]any{
				"type":        "string",
				"description": "搜索引擎：auto（默认，优先 Serper，缺 key 或失败时降级 DuckDuckGo）/serper/duckduckgo",
				"enum":        []string{"auto", "serper", "duckduckgo"},
			},
		},
		"required": []string{"query"},
	}
}

// Execute 解析参数后调用对应搜索引擎，并把结果格式化成纯文本返回给 LLM。
func (w *WebSearchTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	query, ok := args["query"].(string)
	if !ok {
		return "", fmt.Errorf("缺少 query 参数")
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return "", fmt.Errorf("query 不能为空")
	}

	num := 5
	if v, ok := args["num_results"]; ok {
		switch n := v.(type) {
		case float64:
			num = int(n)
		case int:
			num = n
		case int64:
			num = int(n)
		}
	}
	if num < 1 {
		num = 1
	}
	if num > 10 {
		num = 10
	}

	engine := "auto"
	if e, ok := args["engine"].(string); ok && e != "" {
		engine = strings.ToLower(strings.TrimSpace(e))
	}

	switch engine {
	case "serper":
		results, err := w.searchSerper(ctx, query, num)
		if err != nil {
			return "", err
		}
		return formatResults("Serper", query, results), nil
	case "duckduckgo":
		results, err := w.searchDuckDuckGo(ctx, query, num)
		if err != nil {
			return "", err
		}
		return formatResults("DuckDuckGo", query, results), nil
	case "auto", "":
		if w.apiKey != "" {
			if results, err := w.searchSerper(ctx, query, num); err == nil {
				return formatResults("Serper", query, results), nil
			}
			// 降级：Serper 失败继续尝试 DuckDuckGo
		}
		results, err := w.searchDuckDuckGo(ctx, query, num)
		if err != nil {
			return "", err
		}
		return formatResults("DuckDuckGo", query, results), nil
	default:
		return "", fmt.Errorf("未知 engine: %s", engine)
	}
}

// searchResult 为内部搜索引擎抽象出的结果项。
type searchResult struct {
	Title   string
	Link    string
	Snippet string
}

// --- Serper.dev ---

type serperResponse struct {
	Organic []struct {
		Title   string `json:"title"`
		Link    string `json:"link"`
		Snippet string `json:"snippet"`
	} `json:"organic"`
	AnswerBox *struct {
		Title   string `json:"title"`
		Link    string `json:"link"`
		Snippet string `json:"snippet"`
		Answer  string `json:"answer"`
	} `json:"answerBox"`
}

func (w *WebSearchTool) searchSerper(ctx context.Context, query string, num int) ([]searchResult, error) {
	if w.apiKey == "" {
		return nil, fmt.Errorf("缺少 Serper API Key（设置 SERPER_API_KEY 或显式 WithWebSearchAPIKey）")
	}

	payload := map[string]any{
		"q":   query,
		"num": num,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("构造 Serper 请求体失败: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.serperURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("创建 Serper 请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", w.apiKey)

	resp, err := w.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Serper 请求失败: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		return nil, fmt.Errorf("读取 Serper 响应失败: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Serper 返回非 2xx 状态: HTTP %d，body: %s", resp.StatusCode, truncate(string(data), 256))
	}

	var parsed serperResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("解析 Serper JSON 失败: %w", err)
	}

	results := make([]searchResult, 0, num+1)
	// AnswerBox 视作额外的辅助信息，不计入 num 配额。
	if parsed.AnswerBox != nil {
		ab := parsed.AnswerBox
		snippet := strings.TrimSpace(ab.Answer)
		if snippet == "" {
			snippet = strings.TrimSpace(ab.Snippet)
		}
		title := strings.TrimSpace(ab.Title)
		if title == "" {
			title = "Answer Box"
		}
		if snippet != "" || ab.Link != "" {
			results = append(results, searchResult{
				Title:   title,
				Link:    strings.TrimSpace(ab.Link),
				Snippet: snippet,
			})
		}
	}

	added := 0
	for _, item := range parsed.Organic {
		if added >= num {
			break
		}
		results = append(results, searchResult{
			Title:   strings.TrimSpace(item.Title),
			Link:    strings.TrimSpace(item.Link),
			Snippet: strings.TrimSpace(item.Snippet),
		})
		added++
	}

	return results, nil
}

// --- DuckDuckGo HTML ---
//
// DuckDuckGo 没有公开的免费 JSON 搜索 API（Instant Answer API 大部分查询返回空），
// 这里直接抓取 https://duckduckgo.com/html/ 的非 JS 版本，用正则解析每条结果。
// 结构稳定但非官方，故仅作 Serper 不可用时的兜底。

var (
	ddgResultBlock = regexp.MustCompile(`(?s)<a[^>]+class="result__a"[^>]+href="([^"]+)"[^>]*>(.*?)</a>.*?<a[^>]+class="result__snippet"[^>]*>(.*?)</a>`)
	tagStripper    = regexp.MustCompile(`<[^>]+>`)
)

func (w *WebSearchTool) searchDuckDuckGo(ctx context.Context, query string, num int) ([]searchResult, error) {
	form := url.Values{}
	form.Set("q", query)
	form.Set("kl", "wt-wt") // 不限地区

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.duckduckURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("创建 DuckDuckGo 请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// DuckDuckGo 在没有 UA 时会拒绝或返回空结果。
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; GoraBot/1.0)")
	req.Header.Set("Accept", "text/html")

	resp, err := w.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("DuckDuckGo 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("DuckDuckGo 返回非 2xx 状态: HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MiB 上限
	if err != nil {
		return nil, fmt.Errorf("读取 DuckDuckGo 响应失败: %w", err)
	}

	matches := ddgResultBlock.FindAllSubmatch(data, num)
	if len(matches) == 0 {
		return nil, fmt.Errorf("DuckDuckGo 未返回可解析结果")
	}

	results := make([]searchResult, 0, len(matches))
	for _, m := range matches {
		link := decodeDDGLink(string(m[1]))
		title := stripHTML(string(m[2]))
		snippet := stripHTML(string(m[3]))
		results = append(results, searchResult{
			Title:   title,
			Link:    link,
			Snippet: snippet,
		})
	}

	return results, nil
}

// decodeDDGLink 把 DuckDuckGo 的跳板链接（//duckduckgo.com/l/?uddg=...）还原成原始 URL。
func decodeDDGLink(raw string) string {
	raw = html.UnescapeString(strings.TrimSpace(raw))
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "//") {
		raw = "https:" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if u := parsed.Query().Get("uddg"); u != "" {
		if decoded, err := url.QueryUnescape(u); err == nil {
			return decoded
		}
		return u
	}
	return raw
}

func stripHTML(s string) string {
	cleaned := tagStripper.ReplaceAllString(s, "")
	cleaned = html.UnescapeString(cleaned)
	cleaned = strings.ReplaceAll(cleaned, " ", " ")
	return strings.TrimSpace(cleaned)
}

// --- formatting ---

func formatResults(engine, query string, results []searchResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "搜索引擎: %s\n查询: %s\n", engine, query)
	if len(results) == 0 {
		b.WriteString("未找到结果。\n")
		return b.String()
	}
	for i, r := range results {
		fmt.Fprintf(&b, "\n[%d] %s\n", i+1, defaultStr(r.Title, "(无标题)"))
		if r.Link != "" {
			fmt.Fprintf(&b, "    链接: %s\n", r.Link)
		}
		if r.Snippet != "" {
			fmt.Fprintf(&b, "    摘要: %s\n", r.Snippet)
		}
	}
	return b.String()
}

func defaultStr(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
