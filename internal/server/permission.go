package server

import (
	"github.com/Serendipity565/gora/internal/agent/core"
)

// PermissionService 把 *core.ToolPermissionGate 包成业务接口，
// 供 controller 在收到前端授权决定时回写。
type PermissionService interface {
	// Gate 返回内部持有的 ToolPermissionGate；上层在创建 Agent 运行上下文时
	// 通过 core.WithToolPermissionGate(ctx, gate) 注入它。
	Gate() *core.ToolPermissionGate
	// Resolve 用前端返回的决定唤醒等待中的 Agent goroutine；
	// 找不到 requestID 时返回 false，controller 据此返回 404。
	Resolve(requestID string, decision core.ToolPermissionDecision) bool
}

type permissionServiceImpl struct {
	gate *core.ToolPermissionGate
}

// NewPermissionService 创建 PermissionService；内部 gate 由 core.NewToolPermissionGate() 创建。
func NewPermissionService() PermissionService {
	return &permissionServiceImpl{
		gate: core.NewToolPermissionGate(),
	}
}

func (s *permissionServiceImpl) Gate() *core.ToolPermissionGate {
	return s.gate
}

func (s *permissionServiceImpl) Resolve(requestID string, decision core.ToolPermissionDecision) bool {
	if s.gate == nil {
		return false
	}
	return s.gate.Resolve(requestID, decision)
}
