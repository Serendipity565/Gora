package logger

import (
	"go.uber.org/zap/zapcore"
)

// Level 是 zapcore.Level 的类型别名，对外屏蔽 zap 实现细节。
type Level = zapcore.Level

const (
	DebugLevel = zapcore.DebugLevel
	InfoLevel  = zapcore.InfoLevel
	WarnLevel  = zapcore.WarnLevel
	ErrorLevel = zapcore.ErrorLevel
	PanicLevel = zapcore.PanicLevel
	FatalLevel = zapcore.FatalLevel
)
