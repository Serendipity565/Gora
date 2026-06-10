package server

import (
	"sort"

	"github.com/Serendipity565/gora/api/response"
	"github.com/Serendipity565/gora/internal/agent/tool"
)

// ToolService 暴露已注册工具的查询能力。
//
// 真正的工具注册由 internal/agent/tool.Registry 完成，这一层只做读视图。
type ToolService interface {
	List() []response.ToolInfo
}

type toolServiceImpl struct {
	registry *tool.Registry
}

// NewToolService 创建一个 ToolService；registry 可为 nil（List 返回空列表）。
func NewToolService(registry *tool.Registry) ToolService {
	return &toolServiceImpl{registry: registry}
}

func (s *toolServiceImpl) List() []response.ToolInfo {
	if s.registry == nil {
		return []response.ToolInfo{}
	}

	tools := s.registry.List()
	infos := make([]response.ToolInfo, 0, len(tools))
	for _, t := range tools {
		infos = append(infos, response.ToolInfo{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  t.Parameters(),
		})
	}
	sort.Slice(infos, func(i, j int) bool {
		return infos[i].Name < infos[j].Name
	})
	return infos
}
