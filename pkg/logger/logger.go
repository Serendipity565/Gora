// Package logger 提供日志抽象，以接口定义通用行为，以 zap 实现具体的日志输出。
package logger

// Logger is logger interface.
// 定义通用接口，可以使用不同的日志库实现
type Logger interface {
	Debug(msg string, fields ...Field)
	Info(msg string, fields ...Field)
	Warn(msg string, fields ...Field)
	Error(msg string, fields ...Field)
	Panic(msg string, fields ...Field)
	Fatal(msg string, fields ...Field)
	Sync() error
}
