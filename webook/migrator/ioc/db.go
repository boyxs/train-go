package ioc

import (
	"github.com/spf13/viper"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/plugin/opentelemetry/tracing"

	"github.com/boyxs/train-go/webook/migrator/repository/dao"
	"github.com/boyxs/train-go/webook/pkg/gormx"
	gormprom "github.com/boyxs/train-go/webook/pkg/gormx/prometheus"
	loggerx "github.com/boyxs/train-go/webook/pkg/logger"
	"github.com/boyxs/train-go/webook/shared/confkey"
)

// InitDB 初始化 migrator 控制库的 GORM 连接。
//
// 与 chat/ioc/db.go 同构：先 AutoMigrate（避免 information_schema 噪音被 OTel 采 span），
// 再注册 Prometheus callback + OTel tracing plugin。
//
// metric 命名按 webook/CLAUDE.md「服务拆分 14 项」硬规则：用 `webook_db_*`（subsystem 维度），
// 不是 `webook_migrator_*`（service 维度由 Prometheus job label 自动注入）。
func InitDB(_ TimezoneReady, l loggerx.LoggerX) *gorm.DB {
	// GORM SQL 日志经 WithContext(ctx) 出，请求内 SQL 自动带 trace.id（见 pkg/gormx.NewGormLogger）
	gormConfig := gorm.Config{
		Logger: gormx.NewGormLogger(l),
	}
	db, err := gorm.Open(mysql.Open(viper.GetString(confkey.DataMySQLDSN)), &gormConfig)
	if err != nil {
		panic("[migrator] failed to connect mysql: " + err.Error())
	}
	if err := gormprom.NewPrometheusBuilder("webook", "db", "query", "DB 查询统计").
		WithCounter().
		WithHistogram().
		WithSummary().
		Register(db); err != nil {
		panic(err)
	}
	if err := dao.InitTable(db); err != nil {
		panic("[migrator] init table: " + err.Error())
	}
	if err := db.Use(tracing.NewPlugin(
		tracing.WithoutMetrics(),
		tracing.WithoutQueryVariables(),
	)); err != nil {
		panic(err)
	}
	return db
}
