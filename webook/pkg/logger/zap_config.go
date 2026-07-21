package logger

import (
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// EcsEncoderConfig 固定 ECS（Elastic Common Schema）键 + epoch_millis 时间，
// 统一各环境 JSON 日志 schema（对齐 ELK；@timestamp 用毫秒戳，对齐项目 int64ms 约定）。
func EcsEncoderConfig() zapcore.EncoderConfig {
	return zapcore.EncoderConfig{
		TimeKey:       "@timestamp",
		LevelKey:      "log.level",
		NameKey:       "log.logger",
		CallerKey:     "log.origin",
		MessageKey:    "message",
		StacktraceKey: "error.stack_trace",
		LineEnding:    zapcore.DefaultLineEnding,
		EncodeLevel:   zapcore.LowercaseLevelEncoder,
		EncodeTime:    zapcore.EpochMillisTimeEncoder,
		EncodeCaller:  zapcore.ShortCallerEncoder,
	}
}

// InitZap 从 viper 的 "logger" + "otel" 段构建标准 *zap.Logger：
//   - dev/prod 预设 → 覆盖 level/encoding/output_paths
//   - encoding=json 时统一 ECS 键 + epoch_millis（local 的 console 保留可读预设）
//   - 附服务身份字段 service.name/version/environment（取自 otel 段，ECS 对齐）
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
