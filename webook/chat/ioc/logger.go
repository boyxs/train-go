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

// InitLogger 与主仓 internal/ioc/logger.go 同源：
// prod.yaml / staging.yaml 用 production logger（Info、json、无 stacktrace），其它用 development（彩色、调用栈）；
// 通过 yaml 的 logger.level.l 覆盖等级；ginx 全局 logger 同步注入。
func InitLogger() logger.LoggerX {
	// 白名单：仅 prod.yaml / staging.yaml 走严格生产配置
	// 其它文件（local.yaml / dev.yaml / test.yaml / 未来新增…）默认进开发模式，避免新增环境忘改判断踩雷
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
	zap.ReplaceGlobals(l)
	l.Sugar().Infof("[chat] logger config: %+v", cfg)
	lx := logger.NewZapLogger(l)
	ginx.L = lx
	interceptor.L = lx
	return lx
}
