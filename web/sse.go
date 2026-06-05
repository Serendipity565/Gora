package web

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/Serendipity565/gora/agent"
)

type sseEventPayload struct {
	Type      string         `json:"type"`
	AgentID   string         `json:"agent_id,omitempty"`
	Content   string         `json:"content,omitempty"`
	Timestamp string         `json:"timestamp,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// SSEWriter 封装 Server-Sent Events 写入逻辑。
type SSEWriter struct {
	w       http.ResponseWriter
	flusher http.Flusher
}

// NewSSEWriter 创建一个 SSE 写入器，并写入对应的响应头。
func NewSSEWriter(w http.ResponseWriter) (*SSEWriter, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("streaming not supported")
	}

	header := w.Header()
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache")
	header.Set("Connection", "keep-alive")
	header.Set("X-Accel-Buffering", "no")
	header.Set("Access-Control-Allow-Origin", "*")

	flusher.Flush()

	return &SSEWriter{w: w, flusher: flusher}, nil
}

// WriteEvent 把单个 Agent 事件序列化为 SSE 帧。
func (s *SSEWriter) WriteEvent(event agent.Event) error {
	data, err := json.Marshal(sseEventPayload{
		Type:      event.Type.String(),
		AgentID:   event.AgentID,
		Content:   event.Content,
		Timestamp: event.Timestamp.Format("2006-01-02T15:04:05.000000000Z07:00"),
		Metadata:  event.Metadata,
	})
	if err != nil {
		return err
	}

	if _, err := fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", event.Type.String(), data); err != nil {
		return err
	}
	s.flusher.Flush()
	return nil
}

// WriteDone 发送一个独立的 done 事件，用于让前端关闭流。
func (s *SSEWriter) WriteDone() error {
	data, err := json.Marshal(sseEventPayload{Type: agent.EventDone.String()})
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(s.w, "event: done\ndata: %s\n\n", data); err != nil {
		return err
	}
	s.flusher.Flush()
	return nil
}

// WriteError 把错误以 SSE 错误事件的形式发送给前端。
func (s *SSEWriter) WriteError(message string) error {
	event := agent.NewErrorEvent("system", fmt.Errorf("%s", message))
	return s.WriteEvent(event)
}
