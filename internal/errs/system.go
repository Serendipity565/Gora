// Package errs 定义业务错误码常量与构造函数，按域划分文件。
package errs

import (
	"net/http"

	"github.com/Serendipity565/gora/pkg/errorx"
)

// 10xxxx - 系统 / 基础设施错误

// 系统级错误码。
const (
	InternalServerErrorCode = 100000 + iota
	SerializationErrorCode
	DeserializationErrorCode
)

var (
	// InternalServerError 内部服务器错误。
	InternalServerError = func(err error) error {
		return errorx.New(http.StatusInternalServerError, InternalServerErrorCode, "服务内部错误", err)
	}
	// SerializationError 序列化错误。
	SerializationError = func(err error) error {
		return errorx.New(http.StatusInternalServerError, SerializationErrorCode, "序列化错误", err)
	}
	// DeserializationError 反序列化错误。
	DeserializationError = func(err error) error {
		return errorx.New(http.StatusInternalServerError, DeserializationErrorCode, "反序列化错误", err)
	}
)
