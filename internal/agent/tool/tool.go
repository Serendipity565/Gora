package tool

import "context"

// Tool 表示 Agent 可以调用的一项外部能力。
type Tool interface {
	Name() string
	Description() string
	Parameters() map[string]any
	Execute(ctx context.Context, args map[string]any) (string, error)
}
