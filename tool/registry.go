package tool

import (
	"fmt"
	"sync"
)

// Registry 管理当前进程中所有可用工具。
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// NewRegistry 创建一个空工具注册表。
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register 把工具注册到当前注册表中。
func (r *Registry) Register(t Tool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := t.Name()
	if _, ok := r.tools[name]; ok {
		return fmt.Errorf("tool %s already registered", name)
	}
	r.tools[name] = t
	return nil
}

// Get 按名称查找工具。
func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	t, ok := r.tools[name]
	return t, ok
}

// List 返回当前已注册的所有工具。
func (r *Registry) List() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tools := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		tools = append(tools, t)
	}
	return tools
}

// ToOpenAISchema 把工具定义转换为 OpenAI/DeepSeek 兼容的工具定义。
func (r *Registry) ToOpenAISchema() []map[string]any {
	tools := r.List()
	schemas := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		schemas = append(schemas, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        t.Name(),
				"description": t.Description(),
				"parameters":  t.Parameters(),
			},
		})
	}
	return schemas
}
