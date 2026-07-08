package ioc

import (
	"github.com/spf13/viper"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/plugin/opentelemetry/tracing"

	"github.com/boyxs/train-go/webook/internal/repository/dao"
	gormprom "github.com/boyxs/train-go/webook/pkg/gormx/prometheus"
	loggerx "github.com/boyxs/train-go/webook/pkg/logger"
	"github.com/boyxs/train-go/webook/shared/confkey"
)

func InitDB(_ TimezoneReady, l loggerx.LoggerX) *gorm.DB {
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
	db, err := gorm.Open(mysql.Open(viper.GetString(confkey.DataMySQLDSN)), &gormConfig)
	// db, err := gorm.Open(mysql.Open(config.Config.MySQL.DSN), &gorm.Config{})
	if err != nil {
		// 数据库都连接不上，就不要启动服务了
		panic("failed to connect database")
	}
	// 注册 Prometheus 指标 callback（Counter + Histogram + Summary）
	if err := gormprom.NewPrometheusBuilder("webook", "db", "query", "DB 查询统计").
		WithCounter().
		WithHistogram().
		WithSummary().
		Register(db); err != nil {
		panic(err)
	}
	// 先跑 AutoMigrate：它会查 information_schema.statistics 等系统表判断索引存在
	// 放在 tracing plugin 注册之前，避免这些启动噪音 SQL 也被采 span
	err = dao.InitTable(db)
	if err != nil {
		panic(err)
	}
	// OTel：每条 SQL 自动产生 span（kind=Client）+ db.statement / db.system 等 semconv 属性
	// 用 WithoutMetrics 避免与 gormprom 重复采集；WithoutQueryVariables 隐藏 SQL 参数防泄敏
	if err := db.Use(tracing.NewPlugin(
		tracing.WithoutMetrics(),
		tracing.WithoutQueryVariables(),
	)); err != nil {
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
