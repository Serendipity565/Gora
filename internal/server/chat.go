// Package server 是 Gora HTTP 接口与底层组件之间的业务编排层。
//
// 这一层对 HTTP / SSE 协议无感知：controller 把请求解析后调用 server，
// server 负责协调 agent / repository / 权限 gate 等组件，并通过事件流回写。
//
// 当 chat 编排逻辑（载入历史、写回历史、流量控制等）需要扩展时，应集中在这里。
package server

import (
	"context"

	"github.com/Serendipity565/gora/internal/agent/core"
)

// AgentRunner 抽象 Agent 运行能力（即可被 server 调用的最小接口）。
type AgentRunner interface {
	ID() string
	State() core.State
	Run(ctx context.Context, input string) <-chan core.Event
}

// SessionRunner 是支持多会话的 Agent。
type SessionRunner interface {
	AgentRunner
	RunSession(ctx context.Context, userID uint64, sessionID, input string) <-chan core.Event
}

// EventSink 是 server 把 Agent 事件流回写到外部（典型为 SSE Writer）的抽象。
type EventSink interface {
	WriteEvent(event core.Event) error
	WriteDone() error
}

// ChatRequest 是 server 内部使用的、与 HTTP 协议解耦的请求结构。
type ChatRequest struct {
	Message       string
	AgentID       string
	SessionID     string
	UserID        uint64
	DisabledTools []string
}

// ErrAgentNotFound 表示找不到匹配 ID 的 Agent。
type ErrAgentNotFound struct{}

func (ErrAgentNotFound) Error() string { return "agent not found" }

// ChatService 编排一次 chat 会话：
//   - 解析期望的 agent + session；
//   - 把 disabledTools 与 permissionGate 注入 context；
//   - 流式消费 Agent 事件并写回 EventSink。
type ChatService interface {
	Stream(ctx context.Context, req ChatRequest, sink EventSink) error
}

type chatServiceImpl struct {
	agentSvc      AgentService
	permissionSvc PermissionService
}

// NewChatService 创建一个 ChatService。
func NewChatService(agentSvc AgentService, permissionSvc PermissionService) ChatService {
	return &chatServiceImpl{
		agentSvc:      agentSvc,
		permissionSvc: permissionSvc,
	}
}

// Stream 执行一次 chat 流式编排，把事件写入 sink。
//   - ctx 已经携带超时；server 仅在其上叠加业务上下文（disabled tools / permission gate）。
//   - 返回的 error 通常是 ErrAgentNotFound 或 sink.WriteEvent 的错误，
//     调用方据此决定 HTTP 层的状态码。
func (s *chatServiceImpl) Stream(ctx context.Context, req ChatRequest, sink EventSink) error {
	a, ok := s.agentSvc.Resolve(req.AgentID)
	if !ok {
		return ErrAgentNotFound{}
	}

	ctx = core.WithDisabledTools(ctx, req.DisabledTools)
	if s.permissionSvc != nil {
		if gate := s.permissionSvc.Gate(); gate != nil {
			ctx = core.WithToolPermissionGate(ctx, gate)
		}
	}

	var events <-chan core.Event
	if sessionRunner, ok := a.(SessionRunner); ok {
		events = sessionRunner.RunSession(ctx, req.UserID, req.SessionID, req.Message)
	} else {
		events = a.Run(ctx, req.Message)
	}

	terminalSent := false
	for event := range events {
		if err := sink.WriteEvent(event); err != nil {
			// 写入失败通常意味着客户端断开。
			return err
		}
		if event.Type == core.EventDone || event.Type == core.EventError {
			terminalSent = true
			break
		}
	}

	if !terminalSent {
		return sink.WriteDone()
	}
	return nil
}
