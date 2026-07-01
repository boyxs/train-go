package ioc

import (
	"path/filepath"

	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/webook/pkg/ginx"
	"github.com/webook/pkg/logger"
)

// InitLogger prod/staging 用 production logger，其它用 development；yaml logger.level.l 覆盖等级。
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
	l.Sugar().Infof("[interaction] logger config: %+v", cfg)
	lx := logger.NewZapLogger(l)
	ginx.L = lx
	return lx
}
