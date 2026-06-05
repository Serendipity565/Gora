package core

import (
	"context"
	"time"
)

// State 表示 Agent 当前所处的生命周期状态。
type State int

const (
	StateIdle    State = iota // 空闲，尚未开始执行
	StateRunning              // 运行中，正在思考或生成结果
	StateWaiting              // 等待中，通常是在等待工具或外部调用返回
	StateDone                 // 已完成，本轮任务正常结束
	StateError                // 出错，本轮任务异常结束
)

func (s State) String() string {
	switch s {
	case StateIdle:
		return "idle"
	case StateRunning:
		return "running"
	case StateWaiting:
		return "waiting"
	case StateDone:
		return "done"
	case StateError:
		return "error"
	default:
		return "unknown"
	}
}

// EventType 表示 Agent 运行过程中向外发出的事件类型。
type EventType int

const (
	EventThinking              EventType = iota // Agent 正在思考
	EventToolCall                               // Agent 正在调用工具
	EventToolResult                             // 工具已返回结果
	EventChunk                                  // LLM 或 Agent 输出的流式片段
	EventDone                                   // 当前轮次已完成
	EventError                                  // 当前轮次发生错误
	EventToolPermissionRequest                  // 请求用户确认是否允许调用某个工具（被前端关闭过）
)

func (e EventType) String() string {
	switch e {
	case EventThinking:
		return "thinking"
	case EventToolCall:
		return "tool_call"
	case EventToolResult:
		return "tool_result"
	case EventChunk:
		return "chunk"
	case EventDone:
		return "done"
	case EventError:
		return "error"
	case EventToolPermissionRequest:
		return "tool_permission_request"
	default:
		return "unknown"
	}
}

// Event 是 Agent 执行期间对外发布的运行时事件。
type Event struct {
	Type      EventType      `json:"type"`
	AgentID   string         `json:"agent_id"`
	Content   string         `json:"content"`
	Timestamp time.Time      `json:"timestamp"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// Agent 是框架中所有智能体需要实现的核心接口。
type Agent interface {
	ID() string
	Run(ctx context.Context, input string) <-chan Event
	State() State
	Stop() error
}
