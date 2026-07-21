package ioc

import (
	"github.com/spf13/viper"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/plugin/opentelemetry/tracing"

	"github.com/boyxs/train-go/webook/chat/repository/dao"
	"github.com/boyxs/train-go/webook/pkg/gormx"
	gormprom "github.com/boyxs/train-go/webook/pkg/gormx/prometheus"
	loggerx "github.com/boyxs/train-go/webook/pkg/logger"
	"github.com/boyxs/train-go/webook/shared/confkey"
)

func InitDB(_ TimezoneReady, l loggerx.LoggerX) *gorm.DB {
	// GORM SQL 日志经 WithContext(ctx) 出，请求内 SQL 自动带 trace.id（见 pkg/gormx.NewGormLogger）
	gormConfig := gorm.Config{
		Logger: gormx.NewGormLogger(l),
	}
	db, err := gorm.Open(mysql.Open(viper.GetString(confkey.DataMySQLDSN)), &gormConfig)
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
