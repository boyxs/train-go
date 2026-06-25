package ioc

import (
	"path/filepath"

	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/webook/pkg/ginx"
	"github.com/webook/pkg/logger"
)

// InitLogger 与 chat/ioc/logger.go 同源。
// prod.yaml / staging.yaml 走严格生产配置（Info、json、无 stacktrace），其它走 development。
// 通过 yaml `logger.level.l` 覆盖等级；ginx.L 全局 logger 同步注入（middleware 错误日志路径）。
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
	l.Sugar().Infof("[migrator] logger config: %+v", cfg)
	lx := logger.NewZapLogger(l)
	ginx.L = lx
	return lx
}
