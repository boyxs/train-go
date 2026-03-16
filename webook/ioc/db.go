package ioc

import (
	"gitee.com/train-cloud/geektime-basic-go/internal/repository/dao"
	loggerx "gitee.com/train-cloud/geektime-basic-go/pkg/logger"
	"github.com/spf13/viper"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func InitDB(l loggerx.LoggerX) *gorm.DB {
	//adapter := &LoggerAdapter{fn: l.Debug}
	adapter := loggerFunc(l.Debug)
	gormConfig := gorm.Config{
		Logger: logger.New(adapter, logger.Config{
			SlowThreshold:             0,
			Colorful:                  true,
			IgnoreRecordNotFoundError: false,
			ParameterizedQueries:      false,
			LogLevel:                  logger.Info,
		}),
	}
	db, err := gorm.Open(mysql.Open(viper.GetString("mysql.dsn")), &gormConfig)
	// db, err := gorm.Open(mysql.Open(config.Config.MySQL.DSN), &gorm.Config{})
	if err != nil {
		// 数据库都连接不上，就不要启动服务了
		panic("failed to connect database")
	}
	err = dao.InitTable(db)
	if err != nil {
		panic(err)
	}
	return db
}

// loggerFunc 函数类型是一等公民
// 使用 loggerFunc(l.Debug) （二者函数签名需相同）做类型转换，调用Printf时就相当于调用l.Debug
// 这样就不用写结构体了
type loggerFunc func(msg string, args ...loggerx.Field)

func (f loggerFunc) Printf(msg string, args ...interface{}) {
	f(msg, loggerx.Field{Key: "args", Val: args})
}

// LoggerAdapter 结构体方式适配
type LoggerAdapter struct {
	fn func(msg string, args ...loggerx.Field)
}

func (a *LoggerAdapter) Printf(msg string, args ...interface{}) {
	a.fn(msg, loggerx.Field{Key: "args", Val: args})
}
