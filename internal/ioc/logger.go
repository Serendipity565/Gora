package ioc

import (
	"github.com/Serendipity565/gora/internal/config"
	"github.com/Serendipity565/gora/pkg/logger"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func InitLogger(conf *config.LogConfig) *zap.Logger {
	level := logger.InfoLevel

	al := zap.NewAtomicLevelAt(level)
	cfg := zap.NewProductionEncoderConfig()
	cfg.EncodeTime = zapcore.RFC3339TimeEncoder

	lumberJackLogger := &lumberjack.Logger{
		Filename:   conf.File,
		MaxSize:    conf.MaxSize,    // 在进行切割之前，日志文件的最大大小（以MB为单位）
		MaxBackups: conf.MaxBackups, // 保留旧文件的最大个数
		MaxAge:     conf.MaxAge,     // 保留旧文件的最大天数
		Compress:   conf.Compress,   // 是否压缩/归档旧文件
	}

	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(cfg),
		zapcore.AddSync(lumberJackLogger),
		al,
	)
	return zap.New(core)
}
