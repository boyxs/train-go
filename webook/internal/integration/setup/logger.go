package setup

import (
	"github.com/webook/pkg/logger"
)

func InitLogger() logger.LoggerX {
	return logger.NewNopLogger()
}
