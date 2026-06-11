package ioc

import (
	"github.com/spf13/viper"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
	"gorm.io/plugin/opentelemetry/tracing"

	"github.com/webook/migrator/repository/dao"
	gormprom "github.com/webook/pkg/gormx/prometheus"
	loggerx "github.com/webook/pkg/logger"
)

// InitDB 初始化 migrator 控制库的 GORM 连接。
//
// 与 chat/ioc/db.go 同构：先 AutoMigrate（避免 information_schema 噪音被 OTel 采 span），
// 再注册 Prometheus callback + OTel tracing plugin。
//
// metric 命名按 webook/CLAUDE.md「服务拆分 14 项」硬规则：用 `webook_db_*`（subsystem 维度），
// 不是 `webook_migrator_*`（service 维度由 Prometheus job label 自动注入）。
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

// loggerFunc gormlogger.Interface 的轻量适配器，把 GORM 内部 Printf 转成 LoggerX.Debug。
type loggerFunc func(msg string, args ...loggerx.Field)

func (f loggerFunc) Printf(msg string, args ...interface{}) {
	f(msg, loggerx.Field{Key: "args", Val: args})
}
