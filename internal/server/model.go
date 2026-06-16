package server

import (
	"context"
	"sync"

	"github.com/Serendipity565/gora/api/response"
)

// ModelInfo 是 Agent 模型选择的内部 DTO，与 api/response.ModelInfo 一致。
//
// 别名暴露在 server 包内方便上层（如 internal/app.Runner）写代码时直接用 server.ModelInfo，
// 不必在 server 与 api/response 之间反复转换。
type ModelInfo = response.ModelInfo

// ModelSelector 抽象 "列出 / 读取 / 切换模型" 的能力，
// 由上层（典型为 internal/app.Runner）实现。
type ModelSelector interface {
	ListModels() []ModelInfo
	CurrentModel(ctx context.Context, userID uint64, sessionID string) (ModelInfo, bool, error)
	SelectModel(ctx context.Context, userID uint64, sessionID, selector string) (ModelInfo, error)
}

// ModelService 把 ModelSelector 包成业务层接口，供 controller 调用。
//
// 当 selector 尚未注入（典型情况：wire 阶段 Runner 还没创建）时所有方法返回
// ErrModelSelectorNotConfigured；上层在创建 Runner 之后用 SetSelector 注入它。
type ModelService interface {
	List() ([]ModelInfo, error)
	Current(ctx context.Context, userID uint64, sessionID string) (ModelInfo, bool, error)
	Select(ctx context.Context, userID uint64, sessionID, selector string) (ModelInfo, error)

	// SetSelector 注入 / 替换底层的 ModelSelector。
	// 传 nil 表示清除（后续调用退化为 ErrModelSelectorNotConfigured）。
	SetSelector(selector ModelSelector)
}

// ErrModelSelectorNotConfigured 当上层未注入 ModelSelector 时所有 ModelService 调用返回此错误。
type ErrModelSelectorNotConfigured struct{}

func (ErrModelSelectorNotConfigured) Error() string { return "model selector not configured" }

type modelServiceImpl struct {
	mu       sync.RWMutex
	selector ModelSelector
}

// NewModelService 创建 ModelService；初始未绑定 ModelSelector，
// 由上层（典型为 app.Run）在创建 Runner 之后通过 SetSelector 注入。
func NewModelService() ModelService {
	return &modelServiceImpl{}
}

func (s *modelServiceImpl) SetSelector(selector ModelSelector) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.selector = selector
}

func (s *modelServiceImpl) List() ([]ModelInfo, error) {
	selector := s.snapshot()
	if selector == nil {
		return nil, ErrModelSelectorNotConfigured{}
	}
	return selector.ListModels(), nil
}

func (s *modelServiceImpl) Current(ctx context.Context, userID uint64, sessionID string) (ModelInfo, bool, error) {
	selector := s.snapshot()
	if selector == nil {
		return ModelInfo{}, false, ErrModelSelectorNotConfigured{}
	}
	return selector.CurrentModel(ctx, userID, sessionID)
}

func (s *modelServiceImpl) Select(ctx context.Context, userID uint64, sessionID, selector string) (ModelInfo, error) {
	current := s.snapshot()
	if current == nil {
		return ModelInfo{}, ErrModelSelectorNotConfigured{}
	}
	return current.SelectModel(ctx, userID, sessionID, selector)
}

func (s *modelServiceImpl) snapshot() ModelSelector {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.selector
}
