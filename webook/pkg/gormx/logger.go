package gormx

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	loggerx "github.com/boyxs/train-go/webook/pkg/logger"
)

// Config 对齐 gorm.io/gorm/logger.Config；不传则用默认（Info / 慢查询 200ms / 忽略 RecordNotFound）。
type Config struct {
	LogLevel                  gormlogger.LogLevel // 低于此级不记，Silent 全关
	SlowThreshold             time.Duration       // 耗时 ≥ 此值升 Warn（0=不判慢查询）
	IgnoreRecordNotFoundError bool                // true 时 ErrRecordNotFound 不当错误
}

// GormLogger 实现 GORM logger.Interface：SQL 经 LoggerX.WithContext(ctx) 结构化输出（含 trace.id），
// 按严重度映射 app 级、由 logger.level 决定是否落盘：正常→Debug、慢查询→Warn、真错误(非 RNF)→Error。
type GormLogger struct {
	l   loggerx.LoggerX
	cfg Config
}

// NewGormLogger 构造 GORM logger；同一入口通吃默认与自定义（可选传一个 Config 覆盖）。
func NewGormLogger(l loggerx.LoggerX, cfg ...Config) gormlogger.Interface {
	c := Config{LogLevel: gormlogger.Info, SlowThreshold: 200 * time.Millisecond, IgnoreRecordNotFoundError: true}
	if len(cfg) > 0 {
		c = cfg[0]
		if c.LogLevel == 0 { // 0 非法（gorm 级别自 Silent=1 起），回落默认
			c.LogLevel = gormlogger.Info
		}
	}
	return &GormLogger{l: l, cfg: c}
}

func (g *GormLogger) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	n := *g
	n.cfg.LogLevel = level
	return &n
}

func (g *GormLogger) Info(ctx context.Context, msg string, args ...any) {
	if g.cfg.LogLevel >= gormlogger.Info {
		g.l.WithContext(ctx).Info(msg, loggerx.Field{Key: "args", Val: args})
	}
}

func (g *GormLogger) Warn(ctx context.Context, msg string, args ...any) {
	if g.cfg.LogLevel >= gormlogger.Warn {
		g.l.WithContext(ctx).Warn(msg, loggerx.Field{Key: "args", Val: args})
	}
}

func (g *GormLogger) Error(ctx context.Context, msg string, args ...any) {
	if g.cfg.LogLevel >= gormlogger.Error {
		g.l.WithContext(ctx).Error(msg, loggerx.Field{Key: "args", Val: args})
	}
}

func (g *GormLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	if g.cfg.LogLevel <= gormlogger.Silent {
		return
	}
	elapsed := time.Since(begin)
	sql, rows := fc()
	l := g.l.WithContext(ctx)
	fs := []loggerx.Field{
		loggerx.String("sql", sql),
		loggerx.Int64("rows", rows),
		loggerx.Int64("elapsed_ms", elapsed.Milliseconds()),
	}
	switch {
	case err != nil && (!g.cfg.IgnoreRecordNotFoundError || !errors.Is(err, gorm.ErrRecordNotFound)):
		l.Error("gorm sql", append(fs, loggerx.Error(err))...)
	case g.cfg.SlowThreshold > 0 && elapsed >= g.cfg.SlowThreshold:
		l.Warn("gorm slow sql", append(fs, loggerx.Int64("slow_threshold_ms", g.cfg.SlowThreshold.Milliseconds()))...)
	default:
		if err != nil {
			fs = append(fs, loggerx.Error(err))
		}
		l.Debug("gorm sql", fs...)
	}
}
