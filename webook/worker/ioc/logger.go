package ioc

import (
	"go.uber.org/zap"

	"github.com/boyxs/train-go/webook/pkg/logger"
)

// InitLogger 从 yaml logger + otel 段构建（见 pkg/logger.InitZap）：统一 ECS json schema + service.* 字段。
// worker 无 ginx web 层，不注入 ginx.L。
func InitLogger() logger.LoggerX {
	l := logger.InitZap()
	zap.ReplaceGlobals(l)
	return logger.NewZapLogger(l)
}
