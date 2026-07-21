package ioc

import (
	"github.com/spf13/viper"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/plugin/opentelemetry/tracing"

	"github.com/boyxs/train-go/webook/comment/repository/dao"
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
