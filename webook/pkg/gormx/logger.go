package gormx

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	loggerx "github.com/boyxs/train-go/webook/pkg/logger"
)

// GormLogger 实现 GORM logger.Interface：SQL / 慢查询 / 错误经 LoggerX.WithContext(ctx) 输出，
// 使请求内的 SQL 日志自动带 trace.id / span.id（GORM 的 Trace/Info/Warn/Error 都收 ctx）；
// 启动期 AutoMigrate 的 ctx 无 span → 自然不带 trace（正确）。
//
// 行为对齐原 gormlogger.New(loggerFunc(l.Debug), Config{LogLevel:Info}):正常 SQL 走 Debug 级
// （local/dev 的 logger.level=debug 可见、prod=info 静默），但字段结构化（sql/rows/elapsed_ms）
// 替代原来的颜色转义格式串，便于 ELK/Kibana 检索。
// 例外：真正的 DB 错误（非 ErrRecordNotFound）升到 Error 级，确保 prod(info) 也能在 ELK 看到 SQL 失败。
type GormLogger struct {
	l     loggerx.LoggerX
	level gormlogger.LogLevel
}

// NewGormLogger 构造 GORM logger（默认 Info 级，对齐各服务 ioc/db.go 原配置）。
func NewGormLogger(l loggerx.LoggerX) gormlogger.Interface {
	return &GormLogger{l: l, level: gormlogger.Info}
}

func (g *GormLogger) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	n := *g
	n.level = level
	return &n
}

func (g *GormLogger) Info(ctx context.Context, msg string, args ...any) {
	if g.level >= gormlogger.Info {
		g.l.WithContext(ctx).Info(msg, loggerx.Field{Key: "args", Val: args})
	}
}

func (g *GormLogger) Warn(ctx context.Context, msg string, args ...any) {
	if g.level >= gormlogger.Warn {
		g.l.WithContext(ctx).Warn(msg, loggerx.Field{Key: "args", Val: args})
	}
}

func (g *GormLogger) Error(ctx context.Context, msg string, args ...any) {
	if g.level >= gormlogger.Error {
		g.l.WithContext(ctx).Error(msg, loggerx.Field{Key: "args", Val: args})
	}
}

func (g *GormLogger) Trace(ctx context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	if g.level <= gormlogger.Silent {
		return
	}
	elapsed := time.Since(begin)
	sql, rows := fc()
	fields := []loggerx.Field{
		loggerx.String("sql", sql),
		loggerx.Int64("rows", rows),
		loggerx.Int64("elapsed_ms", elapsed.Milliseconds()),
	}
	if err != nil {
		fields = append(fields, loggerx.Error(err))
	}
	// 真正的 DB 错误升 Error 级（prod info 也可见）；ErrRecordNotFound 是正常业务流，仍走 Debug。
	// trace.id 由 WithContext(ctx) 注入
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		g.l.WithContext(ctx).Error("gorm sql", fields...)
		return
	}
	// 正常 SQL / ErrRecordNotFound：Debug 级（对齐原 loggerFunc(l.Debug)）
	g.l.WithContext(ctx).Debug("gorm sql", fields...)
}
