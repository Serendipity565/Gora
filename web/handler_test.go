package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Serendipity565/gora/agent"
	"github.com/Serendipity565/gora/tool"
)

type stubSessionAgent struct {
	id          string
	state       agent.State
	lastInput   string
	lastSession string
	runs        int
}

func (a *stubSessionAgent) ID() string {
	return a.id
}

func (a *stubSessionAgent) State() agent.State {
	return a.state
}

func (a *stubSessionAgent) Run(ctx context.Context, input string) <-chan agent.Event {
	return a.RunSession(ctx, "", input)
}

func (a *stubSessionAgent) RunSession(ctx context.Context, sessionID, input string) <-chan agent.Event {
	a.runs++
	a.lastInput = input
	a.lastSession = sessionID

	events := make(chan agent.Event, 3)
	events <- agent.NewThinkingEvent(a.id, "thinking")
	events <- agent.NewChunkEvent(a.id, "hello")
	events <- agent.NewDoneEvent(a.id)
	close(events)
	return events
}

type stubTool struct {
	name string
}

func (t stubTool) Name() string {
	return t.name
}

func (t stubTool) Description() string {
	return t.name + " description"
}

func (t stubTool) Parameters() map[string]any {
	return map[string]any{"type": "object"}
}

func (t stubTool) Execute(context.Context, map[string]any) (string, error) {
	return "", nil
}

func TestHandleChatUsesRouteAgentIDAndSessionRunner(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	handler := NewHandler(tool.NewRegistry())
	routeAgent := &stubSessionAgent{id: "route-agent", state: agent.StateIdle}
	bodyAgent := &stubSessionAgent{id: "body-agent", state: agent.StateIdle}
	handler.RegisterAgent(routeAgent)
	handler.RegisterAgent(bodyAgent)

	router := gin.New()
	router.POST("/api/chat/:agentId", handler.HandleChat)

	req := httptest.NewRequest(http.MethodPost, "/api/chat/route-agent", strings.NewReader(`{"message":"hello","agent_id":"body-agent","session_id":"session-1"}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.Code)
	}
	if got := resp.Header().Get("Content-Type"); !strings.Contains(got, "text/event-stream") {
		t.Fatalf("unexpected content-type: %q", got)
	}
	if routeAgent.runs != 1 {
		t.Fatalf("expected route agent to run once, got %d", routeAgent.runs)
	}
	if bodyAgent.runs != 0 {
		t.Fatalf("expected body agent to not run, got %d", bodyAgent.runs)
	}
	if routeAgent.lastSession != "session-1" {
		t.Fatalf("unexpected session id: %q", routeAgent.lastSession)
	}
	if routeAgent.lastInput != "hello" {
		t.Fatalf("unexpected input: %q", routeAgent.lastInput)
	}

	body := resp.Body.String()
	if count := strings.Count(body, "event: done"); count != 1 {
		t.Fatalf("expected one done event, got %d body=%q", count, body)
	}
	if !strings.Contains(body, `"type":"thinking"`) {
		t.Fatalf("expected thinking payload in body: %q", body)
	}
	if !strings.Contains(body, `"agent_id":"route-agent"`) {
		t.Fatalf("expected route agent id in payload: %q", body)
	}
}

func TestHandleListAgentsSorted(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	handler := NewHandler(nil, WithModelName("deepseek-chat"))
	handler.RegisterAgent(&stubSessionAgent{id: "b-agent", state: agent.StateRunning})
	handler.RegisterAgent(&stubSessionAgent{id: "a-agent", state: agent.StateIdle})

	router := gin.New()
	router.GET("/api/agents", handler.HandleListAgents)

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/agents", nil))

	if resp.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.Code)
	}

	var payload struct {
		Agents []struct {
			ID    string `json:"id"`
			State string `json:"state"`
			Type  string `json:"type"`
			Model string `json:"model"`
		} `json:"agents"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal agents response: %v", err)
	}
	if len(payload.Agents) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(payload.Agents))
	}
	if payload.Agents[0].ID != "a-agent" || payload.Agents[1].ID != "b-agent" {
		t.Fatalf("expected sorted agents, got %#v", payload.Agents)
	}
	if payload.Agents[0].State != "idle" {
		t.Fatalf("unexpected first state: %#v", payload.Agents[0])
	}
	if payload.Agents[0].Type != "unknown" {
		t.Fatalf("unexpected first type: %#v", payload.Agents[0])
	}
	if payload.Agents[0].Model != "deepseek-chat" {
		t.Fatalf("unexpected model: %#v", payload.Agents[0])
	}
}

func TestHandleListToolsSorted(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	registry := tool.NewRegistry()
	if err := registry.Register(stubTool{name: "z-tool"}); err != nil {
		t.Fatalf("register z-tool: %v", err)
	}
	if err := registry.Register(stubTool{name: "a-tool"}); err != nil {
		t.Fatalf("register a-tool: %v", err)
	}

	handler := NewHandler(registry)
	router := gin.New()
	router.GET("/api/tools", handler.HandleListTools)

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/tools", nil))

	if resp.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.Code)
	}

	var payload struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal tools response: %v", err)
	}
	if len(payload.Tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(payload.Tools))
	}
	if payload.Tools[0].Name != "a-tool" || payload.Tools[1].Name != "z-tool" {
		t.Fatalf("expected sorted tools, got %#v", payload.Tools)
	}
}

func TestHandleIndexReturnsEmbeddedPage(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	handler := NewHandler(nil)
	router := gin.New()
	router.GET("/", handler.HandleIndex)

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/", nil))

	if resp.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.Code)
	}
	if !strings.Contains(resp.Body.String(), "<title>Gora Playground</title>") {
		t.Fatalf("expected embedded html title, got %q", resp.Body.String())
	}
}
