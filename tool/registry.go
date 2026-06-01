package tool

import "fmt"

// Registry manages all available tools.
type Registry struct {
	tools map[string]Tool
}

func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

func (r *Registry) Register(t Tool) error {
	name := t.Name()
	if _, ok := r.tools[name]; ok {
		return fmt.Errorf("tool %s already registered", name)
	}
	r.tools[name] = t
	return nil
}

func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

func (r *Registry) List() []Tool {
	tools := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		tools = append(tools, t)
	}
	return tools
}

func (r *Registry) ToOpenAISchema() []map[string]any {
	schemas := make([]map[string]any, 0, len(r.tools))
	for _, t := range r.tools {
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
