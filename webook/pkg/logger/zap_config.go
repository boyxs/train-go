package logger

import (
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// EcsEncoderConfig 固定 ECS 键 + epoch_millis 时间戳（对齐 ELK / 项目 int64ms）。
func EcsEncoderConfig() zapcore.EncoderConfig {
	return zapcore.EncoderConfig{
		TimeKey:    "@timestamp",
		LevelKey:   "log.level",
		NameKey:    "log.logger",
		CallerKey:  "log.origin",
		MessageKey: "message",
		// 堆栈走独立顶层字段，不放 error.stack_trace：否则 error 被当对象，与 logger.Error 写的标量 error 冲突(ES 400)
		StacktraceKey: "stack_trace",
		LineEnding:    zapcore.DefaultLineEnding,
		EncodeLevel:   zapcore.LowercaseLevelEncoder,
		EncodeTime:    zapcore.EpochMillisTimeEncoder,
		EncodeCaller:  zapcore.ShortCallerEncoder,
	}
}

// InitZap 从 viper logger+otel 段构建 *zap.Logger：dev/prod 预设，json 编码时用 ECS 键，
// 附 service.name/version/environment（取自 otel 段）。
func InitZap() *zap.Logger {
	var lc struct {
		Level            string   `mapstructure:"level"`
		Development      bool     `mapstructure:"development"`
		Encoding         string   `mapstructure:"encoding"`
		OutputPaths      []string `mapstructure:"output_paths"`
		ErrorOutputPaths []string `mapstructure:"error_output_paths"`
	}
	if err := viper.UnmarshalKey("logger", &lc); err != nil {
		panic(err)
	}
	var cfg zap.Config
	if lc.Development {
		cfg = zap.NewDevelopmentConfig()
	} else {
		cfg = zap.NewProductionConfig()
	}
	if lc.Level != "" {
		lvl, err := zapcore.ParseLevel(lc.Level)
		if err != nil {
			panic(err)
		}
		cfg.Level.SetLevel(lvl)
	}
	if lc.Encoding != "" {
		cfg.Encoding = lc.Encoding
	}
	if len(lc.OutputPaths) > 0 {
		cfg.OutputPaths = lc.OutputPaths
	}
	if len(lc.ErrorOutputPaths) > 0 {
		cfg.ErrorOutputPaths = lc.ErrorOutputPaths
	}
	if cfg.Encoding == "json" {
		cfg.EncoderConfig = EcsEncoderConfig()
	}
	l, err := cfg.Build()
	if err != nil {
		panic(err)
	}
	var oc struct {
		ServiceName    string `mapstructure:"service_name"`
		ServiceVersion string `mapstructure:"service_version"`
		Env            string `mapstructure:"env"`
	}
	if err := viper.UnmarshalKey("otel", &oc); err != nil {
		panic(err)
	}
	return l.With(
		zap.String("service.name", oc.ServiceName),
		zap.String("service.version", oc.ServiceVersion),
		zap.String("service.environment", oc.Env),
	)
}
