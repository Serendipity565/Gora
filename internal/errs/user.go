package errs

import (
	"net/http"

	"github.com/Serendipity565/gora/pkg/errorx"
)

/*
2001xx - 用户业务错误
*/

const (
	ErrEmailAlreadyUsedCode = 200100 + iota
	ErrUserNotFoundCode
	ErrUserDisabledCode
	ErrInvalidPasswordCode
)

var (
	ErrEmailAlreadyUsed = func(err error) error {
		return errorx.New(http.StatusBadRequest, ErrEmailAlreadyUsedCode, "注册邮箱已被使用", err)
	}
	ErrUserNotFound = func(err error) error {
		return errorx.New(http.StatusNotFound, ErrUserNotFoundCode, "用户不存在", err)
	}
	ErrUserDisabled = func(err error) error {
		return errorx.New(http.StatusForbidden, ErrUserDisabledCode, "用户已被禁用", err)
	}
	ErrInvalidPassword = func(err error) error {
		return errorx.New(http.StatusUnauthorized, ErrInvalidPasswordCode, "密码错误", err)
	}
)
