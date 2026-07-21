package ioc

import (
	"github.com/spf13/viper"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/plugin/opentelemetry/tracing"

	"github.com/boyxs/train-go/webook/pkg/gormx"
	"github.com/boyxs/train-go/webook/relation/repository/dao"

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
