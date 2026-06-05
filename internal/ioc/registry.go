package ioc

import (
	"fmt"

	"github.com/Serendipity565/gora/internal/agent/tool"
	"github.com/Serendipity565/gora/internal/agent/tool/builtin"
)

// NewToolRegistry 创建工具注册表，并把内置工具（HTTP 等）注册进去。
//
// 当前阶段只有 HTTPTool；新增内置工具时在这里集中扩展，避免散落。
func NewToolRegistry() (*tool.Registry, error) {
	registry := tool.NewRegistry()
	if err := registry.Register(builtin.NewHTTPTool()); err != nil {
		return nil, fmt.Errorf("注册内置工具失败: %w", err)
	}
	return registry, nil
}
