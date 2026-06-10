package logger

import (
	"go.uber.org/zap"
)

func NewZapLogger(l *zap.Logger) Logger {
	return &ZapLogger{l: l}
}

type ZapLogger struct {
	l *zap.Logger
}

func (l *ZapLogger) Debug(msg string, fields ...Field) {
	l.l.Debug(msg, fields...)
}

func (l *ZapLogger) Info(msg string, fields ...Field) {
	l.l.Info(msg, fields...)
}

func (l *ZapLogger) Warn(msg string, fields ...Field) {
	l.l.Warn(msg, fields...)
}

func (l *ZapLogger) Error(msg string, fields ...Field) {
	l.l.Error(msg, fields...)
}

func (l *ZapLogger) Panic(msg string, fields ...Field) {
	l.l.Panic(msg, fields...)
}

func (l *ZapLogger) Fatal(msg string, fields ...Field) {
	l.l.Fatal(msg, fields...)
}

func (l *ZapLogger) Sync() error {
	return l.l.Sync()
}
