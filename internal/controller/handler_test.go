package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Serendipity565/gora/internal/agent/core"
	"github.com/Serendipity565/gora/internal/agent/tool"
)

type stubSessionAgent struct {
	id          string
	state       core.State
	lastInput   string
	lastSession string
	runs        int
}

func (a *stubSessionAgent) ID() string {
	return a.id
}

func (a *stubSessionAgent) State() core.State {
	return a.state
}

func (a *stubSessionAgent) Run(ctx context.Context, input string) <-chan core.Event {
	return a.RunSession(ctx, "", input)
}

func (a *stubSessionAgent) RunSession(ctx context.Context, sessionID, input string) <-chan core.Event {
	a.runs++
	a.lastInput = input
	a.lastSession = sessionID

	events := make(chan core.Event, 3)
	events <- core.NewThinkingEvent(a.id, "thinking")
	events <- core.NewChunkEvent(a.id, "hello")
	events <- core.NewDoneEvent(a.id)
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

	h := New(tool.NewRegistry())
	routeAgent := &stubSessionAgent{id: "route-agent", state: core.StateIdle}
	bodyAgent := &stubSessionAgent{id: "body-agent", state: core.StateIdle}
	h.RegisterAgent(routeAgent)
	h.RegisterAgent(bodyAgent)

	router := gin.New()
	router.POST("/api/chat/:agentId", h.HandleChat)

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

	h := New(nil, WithModelName("deepseek-chat"))
	h.RegisterAgent(&stubSessionAgent{id: "b-agent", state: core.StateRunning})
	h.RegisterAgent(&stubSessionAgent{id: "a-agent", state: core.StateIdle})

	router := gin.New()
	router.GET("/api/agents", h.HandleListAgents)

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

	h := New(registry)
	router := gin.New()
	router.GET("/api/tools", h.HandleListTools)

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

func TestHandleToolPermission_ResolvesPendingRequest(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	h := New(nil)
	router := gin.New()
	router.POST("/api/chat/tool-permission", h.HandleToolPermission)

	requestID := core.NewRequestID()
	waitCh, err := h.permissionGate.Register(requestID)
	if err != nil {
		t.Fatalf("register gate: %v", err)
	}

	body := `{"request_id":"` + requestID + `","approve":true,"remember":true}`
	req := httptest.NewRequest(http.MethodPost, "/api/chat/tool-permission", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", resp.Code, resp.Body.String())
	}

	select {
	case decision, ok := <-waitCh:
		if !ok {
			t.Fatal("expected decision on channel, got close")
		}
		if !decision.Approved || !decision.Remember {
			t.Fatalf("unexpected decision: %#v", decision)
		}
	default:
		t.Fatal("expected decision to be delivered immediately")
	}
}

func TestHandleToolPermission_UnknownRequestReturns404(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	h := New(nil)
	router := gin.New()
	router.POST("/api/chat/tool-permission", h.HandleToolPermission)

	req := httptest.NewRequest(http.MethodPost, "/api/chat/tool-permission", strings.NewReader(`{"request_id":"does-not-exist","approve":false}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.Code)
	}
}
