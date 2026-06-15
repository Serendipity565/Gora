package builtin

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebSearchTool_RequiresQuery(t *testing.T) {
	t.Parallel()

	w := NewWebSearchTool()
	if _, err := w.Execute(context.Background(), map[string]any{}); err == nil {
		t.Fatal("expected error when query is missing")
	}
	if _, err := w.Execute(context.Background(), map[string]any{"query": "   "}); err == nil {
		t.Fatal("expected error when query is blank")
	}
}

func TestWebSearchTool_SerperHappyPath(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if got := r.Header.Get("X-API-KEY"); got != "test-key" {
			t.Errorf("expected X-API-KEY=test-key, got %q", got)
		}
		body, _ := io.ReadAll(r.Body)
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("invalid JSON body: %v", err)
		}
		if payload["q"] != "golang generics" {
			t.Errorf("unexpected query: %v", payload["q"])
		}

		rw.Header().Set("Content-Type", "application/json")
		_, _ = rw.Write([]byte(`{
			"answerBox": {"title": "AB", "snippet": "answer-snippet", "link": "https://ab"},
			"organic": [
				{"title": "T1", "link": "https://example.com/1", "snippet": "S1"},
				{"title": "T2", "link": "https://example.com/2", "snippet": "S2"}
			]
		}`))
	}))
	defer server.Close()

	w := NewWebSearchTool(
		WithWebSearchAPIKey("test-key"),
		WithSerperEndpoint(server.URL),
	)
	out, err := w.Execute(context.Background(), map[string]any{
		"query":       "golang generics",
		"num_results": 2,
		"engine":      "serper",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{"Serper", "AB", "answer-snippet", "T1", "https://example.com/1", "T2"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected output to contain %q, got:\n%s", want, out)
		}
	}
}

func TestWebSearchTool_SerperWithoutKeyErrors(t *testing.T) {
	t.Parallel()

	w := NewWebSearchTool(WithWebSearchAPIKey("")) // 显式清空，覆盖可能存在的 env
	_, err := w.Execute(context.Background(), map[string]any{
		"query":  "anything",
		"engine": "serper",
	})
	if err == nil || !strings.Contains(err.Error(), "Serper API Key") {
		t.Fatalf("expected missing api key error, got %v", err)
	}
}

func TestWebSearchTool_DuckDuckGoFallback(t *testing.T) {
	t.Parallel()

	html := `
		<div class="result">
			<a class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com%2Ffoo&rut=abc">Example <b>Foo</b></a>
			<a class="result__snippet" href="//x">snippet for &amp; foo</a>
		</div>
		<div class="result">
			<a class="result__a" href="https://direct.example.com/bar">Bar Title</a>
			<a class="result__snippet" href="//x">bar snippet</a>
		</div>
	`
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		_ = r.ParseForm()
		if r.FormValue("q") != "go" {
			t.Errorf("unexpected q=%q", r.FormValue("q"))
		}
		rw.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(rw, html)
	}))
	defer server.Close()

	w := NewWebSearchTool(
		WithWebSearchAPIKey(""), // 强制走 DDG（auto 模式无 key 即降级）
		WithDuckDuckGoEndpoint(server.URL),
	)
	out, err := w.Execute(context.Background(), map[string]any{
		"query": "go",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "DuckDuckGo") {
		t.Errorf("expected DuckDuckGo header, got:\n%s", out)
	}
	if !strings.Contains(out, "https://example.com/foo") {
		t.Errorf("expected redirected link decoded, got:\n%s", out)
	}
	if !strings.Contains(out, "https://direct.example.com/bar") {
		t.Errorf("expected direct link preserved, got:\n%s", out)
	}
	if !strings.Contains(out, "Example Foo") {
		t.Errorf("expected HTML stripped title, got:\n%s", out)
	}
	if !strings.Contains(out, "snippet for & foo") {
		t.Errorf("expected entity-decoded snippet, got:\n%s", out)
	}
}

func TestWebSearchTool_AutoFallsBackOnSerperError(t *testing.T) {
	t.Parallel()

	serperHits := 0
	serperServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		serperHits++
		http.Error(rw, "boom", http.StatusInternalServerError)
	}))
	defer serperServer.Close()

	ddgHits := 0
	ddgServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		ddgHits++
		_, _ = io.WriteString(rw, `<a class="result__a" href="https://x">X</a><a class="result__snippet">x snippet</a>`)
	}))
	defer ddgServer.Close()

	w := NewWebSearchTool(
		WithWebSearchAPIKey("k"),
		WithSerperEndpoint(serperServer.URL),
		WithDuckDuckGoEndpoint(ddgServer.URL),
	)
	out, err := w.Execute(context.Background(), map[string]any{
		"query":  "fallback test",
		"engine": "auto",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if serperHits != 1 {
		t.Errorf("expected serper to be hit once, got %d", serperHits)
	}
	if ddgHits != 1 {
		t.Errorf("expected ddg to be hit once after serper failed, got %d", ddgHits)
	}
	if !strings.Contains(out, "DuckDuckGo") {
		t.Errorf("expected DuckDuckGo result after fallback, got:\n%s", out)
	}
}

func TestWebSearchTool_UnknownEngine(t *testing.T) {
	t.Parallel()

	w := NewWebSearchTool()
	_, err := w.Execute(context.Background(), map[string]any{
		"query":  "foo",
		"engine": "yahoo",
	})
	if err == nil || !strings.Contains(err.Error(), "未知 engine") {
		t.Fatalf("expected unknown engine error, got %v", err)
	}
}
