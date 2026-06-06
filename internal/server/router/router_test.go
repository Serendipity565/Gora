package router_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Serendipity565/gora/internal/agent/tool"
	"github.com/Serendipity565/gora/internal/server/handler"
	"github.com/Serendipity565/gora/internal/server/router"
)

// 后端是纯 API 服务：/health 与 /api 之外的路径应返回 404。
func TestNewIsAPIOnly(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	h := handler.New(tool.NewRegistry())
	engine := router.New(h, router.Options{})

	healthResp := httptest.NewRecorder()
	engine.ServeHTTP(healthResp, httptest.NewRequest(http.MethodGet, "/health", nil))
	if healthResp.Code != http.StatusOK {
		t.Fatalf("unexpected health status: %d", healthResp.Code)
	}
	if !strings.Contains(healthResp.Body.String(), `"status":"ok"`) {
		t.Fatalf("unexpected health body: %q", healthResp.Body.String())
	}

	rootResp := httptest.NewRecorder()
	engine.ServeHTTP(rootResp, httptest.NewRequest(http.MethodGet, "/", nil))
	if rootResp.Code != http.StatusNotFound {
		t.Fatalf("expected 404 at /, got %d body=%s", rootResp.Code, rootResp.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(rootResp.Body.Bytes(), &body); err != nil {
		t.Fatalf("expected JSON 404 body, got %q", rootResp.Body.String())
	}
	if hint, _ := body["hint"].(string); strings.TrimSpace(hint) == "" {
		t.Fatalf("expected non-empty hint in 404 body, got %#v", body)
	}

	apiMissingResp := httptest.NewRecorder()
	engine.ServeHTTP(apiMissingResp, httptest.NewRequest(http.MethodGet, "/api/does-not-exist", nil))
	if apiMissingResp.Code != http.StatusNotFound {
		t.Fatalf("expected 404 at unknown /api route, got %d", apiMissingResp.Code)
	}
}

func TestNewCORSHeadersWhenEnabled(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	h := handler.New(tool.NewRegistry())
	engine := router.New(h, router.Options{CORS: true})

	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, httptest.NewRequest(http.MethodOptions, "/api/tools", nil))

	if resp.Code != http.StatusNoContent {
		t.Fatalf("expected 204 on preflight, got %d", resp.Code)
	}
	if got := resp.Header().Get("Access-Control-Allow-Origin"); got == "" {
		t.Fatalf("expected CORS allow-origin to be set, got empty")
	}
}
