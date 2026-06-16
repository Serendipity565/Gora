package ioc

import (
	"github.com/Serendipity565/gora/configs"
	"github.com/Serendipity565/gora/pkg/logger"
	"github.com/natefinch/lumberjack"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// InitLogger 根据 cfg 装配 zap + lumberjack 文件切割，并以 logger.Logger 接口返回，
// 直接给 middleware / 业务层注入。
func InitLogger(conf configs.LogConfig) logger.Logger {
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
	return logger.NewZapLogger(zap.New(core))
}
