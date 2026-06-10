package errs

import (
	"net/http"

	"github.com/Serendipity565/gora/pkg/errorx"
)

/*
10xxxx - 系统 / 基础设施错误
*/

const (
	InternalServerErrorCode = 100000 + iota
	SerializationErrorCode
	DeserializationErrorCode
)

var (
	InternalServerError = func(err error) error {
		return errorx.New(http.StatusInternalServerError, InternalServerErrorCode, "服务内部错误", err)
	}
	SerializationError = func(err error) error {
		return errorx.New(http.StatusInternalServerError, SerializationErrorCode, "序列化错误", err)
	}
	DeserializationError = func(err error) error {
		return errorx.New(http.StatusInternalServerError, DeserializationErrorCode, "反序列化错误", err)
	}
)
