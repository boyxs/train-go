package ioc

import (
	"path/filepath"

	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/webook/pkg/logger"
)

// InitLogger 与 chat/internal 同源：prod/staging 用 production logger，其它 development，
// logger.level.l 覆盖等级。worker 无 ginx web 层，不注入 ginx.L。
func InitLogger() logger.LoggerX {
	var cfg zap.Config
	base := filepath.Base(viper.ConfigFileUsed())
	if base == "prod.yaml" || base == "staging.yaml" {
		cfg = zap.NewProductionConfig()
	} else {
		cfg = zap.NewDevelopmentConfig()
	}
	if err := viper.UnmarshalKey("logger", &cfg); err != nil {
		panic(err)
	}
	if viper.IsSet("logger.level.l") {
		cfg.Level.SetLevel(zapcore.Level(viper.GetInt("logger.level.l")))
	}
	l, err := cfg.Build()
	if err != nil {
		panic(err)
	}
	zap.ReplaceGlobals(l)
	return logger.NewZapLogger(l)
}
