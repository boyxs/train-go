package ioc

import (
	"path/filepath"

	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/webook/pkg/ginx"
	"github.com/webook/pkg/grpcx/interceptor"
	"github.com/webook/pkg/logger"
)

func InitLogger() logger.LoggerX {
	// 白名单：仅 prod.yaml / staging.yaml 走严格生产配置（Info 级、json、不带 stacktrace）
	// 其他文件（local.yaml / dev.yaml / test.yaml / 未来新增…）默认进开发模式，避免新增环境忘改判断踩雷
	// 用 filepath.Base 精确匹配文件名，避免目录路径含 "prod"/"staging" 子串误判
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
	// 替换后全局生效
	zap.ReplaceGlobals(l)
	l.Sugar().Infof("logger config: %+v", cfg)
	lx := logger.NewZapLogger(l)
	// 注入 ginx / grpcx 用的全局 logger，让 wrap 和 gRPC interceptor 都用同一个 logger
	ginx.L = lx
	interceptor.L = lx
	return lx
}
