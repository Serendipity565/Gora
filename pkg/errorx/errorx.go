package errorx

import (
	"errors"
	"fmt"
	"net/http"
	"runtime"
)

// CustomError 自定义错误类型
type CustomError struct {
	HttpCode int    `json:"http_code"` // http 错误码
	Code     int    `json:"code"`      // 具体错误码
	Msg      string `json:"message"`   // 暴露给前端的错误信息
	Err      error  `json:"-"`         // 具体错误原因（内部，不暴露）
	File     string `json:"-"`         // 出错的文件名（内部，不暴露）
	Line     int    `json:"-"`         // 出错的行号（内部，不暴露）
	Function string `json:"-"`         // 出错的函数名（内部，不暴露）
}

func (e *CustomError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%d] %s (at %s:%d in %s): %v", e.Code, e.Msg, e.File, e.Line, e.Function, e.Err)
	}
	return fmt.Sprintf("[%d] %s (at %s:%d in %s)", e.Code, e.Msg, e.File, e.Line, e.Function)
}

// New 创建新的 CustomError
func New(httpCode int, code int, message string, err error) error {
	// 获取调用栈信息
	file, line, function := getCallerInfo(3)
	return &CustomError{
		HttpCode: httpCode,
		Code:     code,
		Msg:      message,
		Err:      err,
		File:     file,
		Line:     line,
		Function: function,
	}
}

// getCallerInfo 获取调用信息
func getCallerInfo(skip int) (string, int, string) {
	// skip: 调用栈层级，1 表示当前函数，2 表示上层调用函数 3表示上上级函数
	pc, file, line, ok := runtime.Caller(skip)
	if !ok {
		return "unknown", 0, "unknown"
	}
	function := runtime.FuncForPC(pc).Name()
	return file, line, function
}

// ToCustomError 尝试将 error 转换为 CustomError
func ToCustomError(err error) *CustomError {
	var customErr *CustomError
	if errors.As(err, &customErr) {
		return customErr
	}

	// 如果不是 CustomError 类型，返回一个通用的内部错误
	// skip=4 是为了获取 ToCustomError 的调用者的调用者的调用者的信息，
	// 即原始错误发生的位置。调用栈如下：
	// 0: runtime.Caller
	// 1: getCallerInfo
	// 2: ToCustomError
	// 3: 调用 ToCustomError 的函数
	// 4: 调用该函数的上层（即原始错误发生处）
	file, line, function := getCallerInfo(4)
	return &CustomError{
		HttpCode: http.StatusInternalServerError,
		Code:     50001, // 通用内部错误码
		Msg:      "服务器内部错误",
		Err:      err,
		File:     file,
		Line:     line,
		Function: function,
	}
}
