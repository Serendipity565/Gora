package cmd

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Serendipity565/gora/tool"
	"github.com/Serendipity565/gora/web"
)

func TestNewRouterServesEmbeddedIndexAndHealth(t *testing.T) {
	t.Parallel()

	handler := web.NewHandler(tool.NewRegistry())
	router := newRouter(handler, serveOptions{})

	rootResp := httptest.NewRecorder()
	router.ServeHTTP(rootResp, httptest.NewRequest(http.MethodGet, "/", nil))
	if rootResp.Code != http.StatusOK {
		t.Fatalf("unexpected root status: %d", rootResp.Code)
	}
	if !strings.Contains(rootResp.Body.String(), "<title>Gora Playground</title>") {
		t.Fatalf("expected embedded index, got %q", rootResp.Body.String())
	}

	fallbackResp := httptest.NewRecorder()
	router.ServeHTTP(fallbackResp, httptest.NewRequest(http.MethodGet, "/playground", nil))
	if fallbackResp.Code != http.StatusOK {
		t.Fatalf("unexpected fallback status: %d", fallbackResp.Code)
	}
	if !strings.Contains(fallbackResp.Body.String(), "Gin + SSE Playground") {
		t.Fatalf("expected embedded fallback page, got %q", fallbackResp.Body.String())
	}

	healthResp := httptest.NewRecorder()
	router.ServeHTTP(healthResp, httptest.NewRequest(http.MethodGet, "/health", nil))
	if healthResp.Code != http.StatusOK {
		t.Fatalf("unexpected health status: %d", healthResp.Code)
	}
	if !strings.Contains(healthResp.Body.String(), `"status":"ok"`) {
		t.Fatalf("unexpected health body: %q", healthResp.Body.String())
	}
}

func TestNormalizeAddrAndCleanRelativePath(t *testing.T) {
	t.Parallel()

	if got := normalizeAddr("8081"); got != ":8081" {
		t.Fatalf("unexpected normalized addr: %q", got)
	}
	if got := normalizeAddr(" 0.0.0.0:9000 "); got != "0.0.0.0:9000" {
		t.Fatalf("unexpected preserved addr: %q", got)
	}
	if got := cleanRelativePath("../../etc/passwd"); strings.Contains(got, "..") {
		t.Fatalf("expected cleaned path without parent traversal, got %q", got)
	}
}
