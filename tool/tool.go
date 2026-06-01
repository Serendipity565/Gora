package tool

import "context"

// Tool is an executable capability that an Agent can call.
type Tool interface {
	Name() string
	Description() string
	Parameters() map[string]any
	Execute(ctx context.Context, args map[string]any) (string, error)
}
