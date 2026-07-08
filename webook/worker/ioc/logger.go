package ioc

import (
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/boyxs/train-go/webook/pkg/logger"
	"github.com/boyxs/train-go/webook/shared/confkey"
)

// InitLogger 由 yaml logger 段驱动:development 选 dev/prod base,再按 level/encoding/output 覆盖。
// worker 无 ginx web 层,不注入 ginx.L。level 非法交 zapcore.ParseLevel 自然报错。
func InitLogger() logger.LoggerX {
	var lc struct {
		Level            string   `mapstructure:"level"`
		Development      bool     `mapstructure:"development"`
		Encoding         string   `mapstructure:"encoding"`
		OutputPaths      []string `mapstructure:"output_paths"`
		ErrorOutputPaths []string `mapstructure:"error_output_paths"`
	}
	if err := viper.UnmarshalKey(confkey.Logger, &lc); err != nil {
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
	l, err := cfg.Build()
	if err != nil {
		panic(err)
	}
	zap.ReplaceGlobals(l)
	return logger.NewZapLogger(l)
}
