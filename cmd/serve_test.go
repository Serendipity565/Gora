package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Serendipity565/gora/tool"
	"github.com/Serendipity565/gora/web"
)

// 后端是纯 API 服务：/health 与 /api 之外的路径应当返回 404，
// 并提示用户去前端项目启动 UI。
func TestNewRouterIsAPIOnly(t *testing.T) {
	t.Parallel()

	handler := web.NewHandler(tool.NewRegistry())
	router := newRouter(handler, serveOptions{})

	healthResp := httptest.NewRecorder()
	router.ServeHTTP(healthResp, httptest.NewRequest(http.MethodGet, "/health", nil))
	if healthResp.Code != http.StatusOK {
		t.Fatalf("unexpected health status: %d", healthResp.Code)
	}
	if !strings.Contains(healthResp.Body.String(), `"status":"ok"`) {
		t.Fatalf("unexpected health body: %q", healthResp.Body.String())
	}

	rootResp := httptest.NewRecorder()
	router.ServeHTTP(rootResp, httptest.NewRequest(http.MethodGet, "/", nil))
	if rootResp.Code != http.StatusNotFound {
		t.Fatalf("expected 404 at /, got %d body=%s", rootResp.Code, rootResp.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(rootResp.Body.Bytes(), &body); err != nil {
		t.Fatalf("expected JSON 404 body, got %q", rootResp.Body.String())
	}
	if hint, _ := body["hint"].(string); !strings.Contains(hint, "frontend") {
		t.Fatalf("expected hint pointing to frontend, got %#v", body)
	}

	apiMissingResp := httptest.NewRecorder()
	router.ServeHTTP(apiMissingResp, httptest.NewRequest(http.MethodGet, "/api/does-not-exist", nil))
	if apiMissingResp.Code != http.StatusNotFound {
		t.Fatalf("expected 404 at unknown /api route, got %d", apiMissingResp.Code)
	}
}

func TestNewRouterCORSHeadersWhenEnabled(t *testing.T) {
	t.Parallel()

	handler := web.NewHandler(tool.NewRegistry())
	router := newRouter(handler, serveOptions{CORS: true})

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodOptions, "/api/tools", nil))

	if resp.Code != http.StatusNoContent {
		t.Fatalf("expected 204 on preflight, got %d", resp.Code)
	}
	if got := resp.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("expected CORS allow-origin *, got %q", got)
	}
}

func TestNormalizeAddr(t *testing.T) {
	t.Parallel()

	if got := normalizeAddr("8081"); got != ":8081" {
		t.Fatalf("unexpected normalized addr: %q", got)
	}
	if got := normalizeAddr(" 0.0.0.0:9000 "); got != "0.0.0.0:9000" {
		t.Fatalf("unexpected preserved addr: %q", got)
	}
	if got := normalizeAddr(""); got != ":8080" {
		t.Fatalf("expected default :8080, got %q", got)
	}
}
