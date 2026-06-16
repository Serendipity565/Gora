package errs

import (
	"net/http"

	"github.com/Serendipity565/gora/pkg/errorx"
)

// 2003xx - Session / Message / 历史 业务错误

const (
	ErrSessionNotFoundCode = 200300 + iota
	ErrSessionForbiddenCode
)

var (
	// ErrSessionNotFound 会话不存在。
	ErrSessionNotFound = func(err error) error {
		return errorx.New(http.StatusNotFound, ErrSessionNotFoundCode, "会话不存在", err)
	}
	// ErrSessionForbidden 无权访问该会话。
	ErrSessionForbidden = func(err error) error {
		return errorx.New(http.StatusForbidden, ErrSessionForbiddenCode, "无权访问该会话", err)
	}
)
