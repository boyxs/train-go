package ioc

import (
	"strings"

	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/webook/pkg/ginx"
	"github.com/webook/pkg/logger"
)

func InitLogger() logger.LoggerX {
	var cfg zap.Config
	if strings.Contains(viper.ConfigFileUsed(), "dev") {
		cfg = zap.NewDevelopmentConfig()
	} else {
		cfg = zap.NewProductionConfig()
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
	// 替换后全局生效
	zap.ReplaceGlobals(l)
	l.Sugar().Infof("logger config: %+v", cfg)
	lx := logger.NewZapLogger(l)
	// 注入 ginx wrapper 用的全局 logger
	ginx.L = lx
	return lx
}
