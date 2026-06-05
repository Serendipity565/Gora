// Package request 是 v1 API 的请求体定义。
package request

// Chat 描述一次对话请求。
type Chat struct {
	Message   string `json:"message" binding:"required"`
	AgentID   string `json:"agent_id"`
	SessionID string `json:"session_id"`
	// DisabledTools 是用户在前端 UI 中关闭的工具名集合，
	// 服务端会通过 context 把它透传给 Agent，命中名单的工具会被直接拒绝执行。
	DisabledTools []string `json:"disabled_tools,omitempty"`
}

// ModelSelect 描述一次模型切换请求。
type ModelSelect struct {
	// SessionID 为空时使用默认会话。
	SessionID string `json:"session_id"`
	// Selector 可以是序号（"1"）、name 或 model 字符串。
	Selector string `json:"selector" binding:"required"`
}

// ToolPermission 描述前端确认/拒绝某次工具授权的请求体。
type ToolPermission struct {
	RequestID string `json:"request_id" binding:"required"`
	Approve   bool   `json:"approve"`
	Remember  bool   `json:"remember,omitempty"`
}
