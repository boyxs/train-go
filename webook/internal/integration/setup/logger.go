package setup

import (
	"gitee.com/train-cloud/geektime-basic-go/pkg/logger"
)

func InitLogger() logger.LoggerX {
	return logger.NewNopLogger()
}
