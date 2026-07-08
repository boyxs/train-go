package setup

import (
	"github.com/boyxs/train-go/webook/pkg/logger"
)

func InitLogger() logger.LoggerX {
	return logger.NewNopLogger()
}
