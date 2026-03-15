package ioc

import (
	"strings"

	"gitee.com/train-cloud/geektime-basic-go/pkg/logger"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
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
	return logger.NewZapLogger(l)
}
