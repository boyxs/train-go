package ioc

import (
	"go.uber.org/zap"

	"github.com/boyxs/train-go/webook/pkg/ginx"
	"github.com/boyxs/train-go/webook/pkg/logger"
)

// InitLogger 从 yaml logger + otel 段构建（见 pkg/logger.InitZap）：
// 统一 ECS json schema + service.* 字段；ginx 全局 logger 同步注入（与兄弟服务同构）。
func InitLogger() logger.LoggerX {
	l := logger.InitZap()
	zap.ReplaceGlobals(l)
	lx := logger.NewZapLogger(l)
	ginx.L = lx
	return lx
}
