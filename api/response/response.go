// Package response 是 v1 API 的响应体定义。
package response

import "github.com/Serendipity565/gora/internal/agent/core"

type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"msg"`
	Data    interface{} `json:"data"`
}

// AgentInfo 是返回给前端的 Agent 元信息。
type AgentInfo struct {
	ID    string     `json:"id"`
	State core.State `json:"-"`
	// 状态以字符串形式输出，方便前端渲染。
	StateText string `json:"state"`
	Type      string `json:"type"`
	Model     string `json:"model,omitempty"`
}

// ToolInfo 描述前端可见的工具元信息。
type ToolInfo struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

// ModelInfo 是前端可见的可选模型项。
type ModelInfo struct {
	Index    int    `json:"index"`
	Name     string `json:"name,omitempty"`
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model"`
	Display  string `json:"display"`
}
