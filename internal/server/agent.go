package server

import (
	"sort"
	"sync"

	"github.com/Serendipity565/gora/api/response"
	"github.com/Serendipity565/gora/internal/agent/core"
	"github.com/Serendipity565/gora/internal/agent/eino"
)

// AgentService 维护可运行 Agent 的注册表，并对外提供查询能力。
//
// controller 层只通过 AgentService 暴露的方法访问 Agent，不直接持有 agents map。
type AgentService interface {
	// Register 把一个 Agent 加入注册表，可热替换同 ID 的 Agent。
	Register(a AgentRunner)
	// SetModel 在运行时更新当前默认模型名（用于 /api/agents 元数据）。
	SetModel(model string)
	// List 返回所有 Agent 元信息，按 ID 升序。
	List() []response.AgentInfo
	// Get 返回单个 Agent 元信息；不存在时 ok=false。
	Get(id string) (response.AgentInfo, bool)
	// Resolve 在 agents 中查找一个 Agent；id 为空时返回任一 Agent。
	Resolve(id string) (AgentRunner, bool)
}

type agentServiceImpl struct {
	mu     sync.RWMutex
	agents map[string]AgentRunner
	model  string
}

// NewAgentService 创建一个空的 AgentService。
func NewAgentService() AgentService {
	return &agentServiceImpl{
		agents: make(map[string]AgentRunner),
	}
}

func (s *agentServiceImpl) Register(a AgentRunner) {
	if a == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.agents[a.ID()] = a
}

func (s *agentServiceImpl) SetModel(model string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.model = model
}

func (s *agentServiceImpl) List() []response.AgentInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	infos := make([]response.AgentInfo, 0, len(s.agents))
	for _, a := range s.agents {
		infos = append(infos, s.toAgentInfo(a))
	}
	sort.Slice(infos, func(i, j int) bool {
		return infos[i].ID < infos[j].ID
	})
	return infos
}

func (s *agentServiceImpl) Get(id string) (response.AgentInfo, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.agents[id]
	if !ok {
		return response.AgentInfo{}, false
	}
	return s.toAgentInfo(a), true
}

func (s *agentServiceImpl) Resolve(id string) (AgentRunner, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if id != "" {
		a, ok := s.agents[id]
		return a, ok
	}
	for _, a := range s.agents {
		return a, true
	}
	return nil, false
}

// toAgentInfo 调用方需自行加读锁；模型字段从 s.model 取。
func (s *agentServiceImpl) toAgentInfo(a AgentRunner) response.AgentInfo {
	state := a.State()
	return response.AgentInfo{
		ID:        a.ID(),
		State:     state,
		StateText: state.String(),
		Type:      detectAgentType(a),
		Model:     s.model,
	}
}

// detectAgentType 通过运行期类型断言识别 Agent 实现，仅用于前端展示。
func detectAgentType(a AgentRunner) string {
	if _, ok := a.(*eino.EinoAgent); ok {
		return "eino"
	}
	if _, ok := a.(*core.BaseAgent); ok {
		return "base"
	}
	return "unknown"
}
