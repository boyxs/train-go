package ioc

import (
	"github.com/spf13/viper"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/plugin/opentelemetry/tracing"

	"github.com/webook/relation/repository/dao"

	gormprom "github.com/webook/pkg/gormx/prometheus"
	loggerx "github.com/webook/pkg/logger"
	"github.com/webook/shared/confkey"
)

func InitDB(_ TimezoneReady, l loggerx.LoggerX) *gorm.DB {
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
	// AutoMigrate 放在 tracing plugin 注册之前，避免系统表探测 SQL 被采 span
	if err := dao.InitTable(db); err != nil {
		panic(err)
	}
	if err := db.Use(tracing.NewPlugin(
		tracing.WithoutMetrics(),
		tracing.WithoutQueryVariables(),
	)); err != nil {
		panic(err)
	}
	return db
}

// loggerFunc 函数类型适配 gorm logger.Writer，调用 Printf 即转调 l.Debug，免写结构体。
type loggerFunc func(msg string, args ...loggerx.Field)

func (f loggerFunc) Printf(msg string, args ...interface{}) {
	f(msg, loggerx.Field{Key: "args", Val: args})
}
