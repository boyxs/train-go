package setup

import "github.com/boyxs/train-go/webook/pkg/logger"

// InitLogger 集成测试用 NopLogger（不污染输出），供 repository 装配。
func InitLogger() logger.LoggerX {
	return logger.NewNopLogger()
}
