package gormx_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/boyxs/train-go/webook/pkg/gormx"
	"github.com/boyxs/train-go/webook/pkg/logger"
)

// recordLogger 记录每次日志调用的级别与字段，供断言 gormx 的级别映射。
type recordLogger struct {
	calls *[]logCall
}

type logCall struct {
	level  string
	msg    string
	fields []logger.Field
}

func (r recordLogger) record(level, msg string, fields []logger.Field) {
	*r.calls = append(*r.calls, logCall{level: level, msg: msg, fields: fields})
}

func (r recordLogger) Debug(msg string, fields ...logger.Field) { r.record("debug", msg, fields) }
func (r recordLogger) Info(msg string, fields ...logger.Field)  { r.record("info", msg, fields) }
func (r recordLogger) Warn(msg string, fields ...logger.Field)  { r.record("warn", msg, fields) }
func (r recordLogger) Error(msg string, fields ...logger.Field) { r.record("error", msg, fields) }

func (r recordLogger) WithContext(context.Context) logger.LoggerX { return r }

func fakeSQL(sql string, rows int64) func() (string, int64) {
	return func() (string, int64) { return sql, rows }
}

func hasErrorField(fields []logger.Field) bool {
	for _, f := range fields {
		if f.Key == "error" {
			return true
		}
	}
	return false
}

// Trace 的级别映射：真正的 DB 错误升 Error，ErrRecordNotFound 与正常 SQL 走 Debug。
func TestGormLogger_Trace_LevelMapping(t *testing.T) {
	testCases := []struct {
		name         string
		err          error
		wantLevel    string
		wantErrField bool
	}{
		{name: "真正的 DB 错误升 Error", err: errors.New("connection refused"), wantLevel: "error", wantErrField: true},
		{name: "ErrRecordNotFound 走 Debug", err: gorm.ErrRecordNotFound, wantLevel: "debug", wantErrField: true},
		{name: "正常 SQL 走 Debug", err: nil, wantLevel: "debug", wantErrField: false},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var calls []logCall
			l := gormx.NewGormLogger(recordLogger{calls: &calls})
			l.Trace(context.Background(), time.Now(), fakeSQL("SELECT 1", 1), tc.err)

			assert.Len(t, calls, 1)
			assert.Equal(t, tc.wantLevel, calls[0].level)
			assert.Equal(t, "gorm sql", calls[0].msg)
			assert.Equal(t, tc.wantErrField, hasErrorField(calls[0].fields))
		})
	}
}

// Silent 级别下 Trace 完全静默（含真正的错误也不输出）。
func TestGormLogger_Trace_Silent(t *testing.T) {
	var calls []logCall
	l := gormx.NewGormLogger(recordLogger{calls: &calls}).LogMode(gormlogger.Silent)
	l.Trace(context.Background(), time.Now(), fakeSQL("SELECT 1", 1), errors.New("boom"))
	assert.Empty(t, calls)
}

// 慢查询（耗时 ≥ 阈值）升 Warn。
func TestGormLogger_Trace_SlowQuery(t *testing.T) {
	var calls []logCall
	l := gormx.NewGormLogger(recordLogger{calls: &calls}) // 默认阈值 200ms
	l.Trace(context.Background(), time.Now().Add(-300*time.Millisecond), fakeSQL("SELECT sleep(1)", 1), nil)

	assert.Len(t, calls, 1)
	assert.Equal(t, "warn", calls[0].level)
	assert.Equal(t, "gorm slow sql", calls[0].msg)
}

// IgnoreRecordNotFoundError=false 时 ErrRecordNotFound 也升 Error（扩展配置）。
func TestGormLogger_Trace_RecordNotFoundAsError(t *testing.T) {
	var calls []logCall
	l := gormx.NewGormLogger(recordLogger{calls: &calls}, gormx.Config{IgnoreRecordNotFoundError: false})
	l.Trace(context.Background(), time.Now(), fakeSQL("SELECT 1", 0), gorm.ErrRecordNotFound)

	assert.Len(t, calls, 1)
	assert.Equal(t, "error", calls[0].level)
}
