package ioc

import (
	"github.com/spf13/viper"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
	"gorm.io/plugin/opentelemetry/tracing"

	"github.com/webook/chat/repository/dao"
	gormprom "github.com/webook/pkg/gormx/prometheus"
	loggerx "github.com/webook/pkg/logger"
)

func InitDB(_ TimezoneReady, l loggerx.LoggerX) *gorm.DB {
	adapter := loggerFunc(l.Debug)
	gormConfig := gorm.Config{
		Logger: gormlogger.New(adapter, gormlogger.Config{
			SlowThreshold:             0,
			Colorful:                  true,
			IgnoreRecordNotFoundError: false,
			ParameterizedQueries:      false,
			LogLevel:                  gormlogger.Info,
		}),
	}
	db, err := gorm.Open(mysql.Open(viper.GetString("mysql.dsn")), &gormConfig)
	if err != nil {
		panic("[chat] failed to connect mysql: " + err.Error())
	}
	// Prometheus 指标 callback
	if err := gormprom.NewPrometheusBuilder("webook", "db", "query", "DB 查询统计").
		WithCounter().
		WithHistogram().
		WithSummary().
		Register(db); err != nil {
		panic(err)
	}
	// 先跑 AutoMigrate（避免 information_schema 噪音被采 span）
	if err := dao.InitTable(db); err != nil {
		panic("[chat] init table: " + err.Error())
	}
	// OTel：每条 SQL 自动产生 span
	if err := db.Use(tracing.NewPlugin(
		tracing.WithoutMetrics(),
		tracing.WithoutQueryVariables(),
	)); err != nil {
		panic(err)
	}
	return db
}

type loggerFunc func(msg string, args ...loggerx.Field)

func (f loggerFunc) Printf(msg string, args ...interface{}) {
	f(msg, loggerx.Field{Key: "args", Val: args})
}
