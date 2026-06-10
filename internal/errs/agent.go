package errs

import (
	"net/http"

	"github.com/Serendipity565/gora/pkg/errorx"
)

/*
2002xx - Agent / 工具 / 模型 / 权限 业务错误
*/

const (
	ErrAgentNotFoundCode = 200200 + iota
	ErrModelInvalidCode
	ErrModelSelectorNotConfiguredCode
	ErrPermissionRequestNotFoundCode
)

var (
	ErrAgentNotFound = func(err error) error {
		return errorx.New(http.StatusNotFound, ErrAgentNotFoundCode, "agent 不存在", err)
	}
	ErrModelInvalid = func(err error) error {
		return errorx.New(http.StatusBadRequest, ErrModelInvalidCode, "模型选择无效", err)
	}
	ErrModelSelectorNotConfigured = func(err error) error {
		return errorx.New(http.StatusNotImplemented, ErrModelSelectorNotConfiguredCode, "未配置模型选择器", err)
	}
	ErrPermissionRequestNotFound = func(err error) error {
		return errorx.New(http.StatusNotFound, ErrPermissionRequestNotFoundCode, "授权请求已过期或不存在", err)
	}
)
