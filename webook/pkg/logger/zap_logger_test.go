package logger_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/boyxs/train-go/webook/pkg/logger"
)

// spanCtx 造一个带有效 SpanContext 的 ctx，返回 ctx 及期望的 trace/span 十六进制串。
func spanCtx(t *testing.T) (ctx context.Context, wantTrace, wantSpan string) {
	t.Helper()
	traceID, err := trace.TraceIDFromHex("0102030405060708090a0b0c0d0e0f10")
	assert.NoError(t, err)
	spanID, err := trace.SpanIDFromHex("0102030405060708")
	assert.NoError(t, err)
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	})
	return trace.ContextWithSpanContext(context.Background(), sc), traceID.String(), spanID.String()
}

// newObserved 造一个 observer-backed LoggerX，返回它与捕获日志的句柄。
func newObserved() (logger.LoggerX, *observer.ObservedLogs) {
	core, logs := observer.New(zapcore.InfoLevel)
	return logger.NewZapLogger(zap.New(core)), logs
}

func TestZapLogger_WithContext(t *testing.T) {
	t.Run("有有效 span：注入 trace.id/span.id 且保留业务字段", func(t *testing.T) {
		ctx, wantTrace, wantSpan := spanCtx(t)
		l, logs := newObserved()

		l.WithContext(ctx).Info("hi", logger.String("k", "v"))

		entries := logs.All()
		assert.Len(t, entries, 1)
		fields := entries[0].ContextMap()
		assert.Equal(t, wantTrace, fields["trace.id"])
		assert.Equal(t, wantSpan, fields["span.id"])
		assert.Equal(t, "v", fields["k"])
	})

	t.Run("无 span：不注入 trace.id/span.id", func(t *testing.T) {
		l, logs := newObserved()

		l.WithContext(context.Background()).Info("hi", logger.String("k", "v"))

		fields := logs.All()[0].ContextMap()
		_, hasTrace := fields["trace.id"]
		_, hasSpan := fields["span.id"]
		assert.False(t, hasTrace)
		assert.False(t, hasSpan)
		assert.Equal(t, "v", fields["k"])
	})

	t.Run("invalid span context：不注入", func(t *testing.T) {
		l, logs := newObserved()
		ctx := trace.ContextWithSpanContext(context.Background(), trace.SpanContext{})

		l.WithContext(ctx).Error("boom")

		_, hasTrace := logs.All()[0].ContextMap()["trace.id"]
		assert.False(t, hasTrace)
	})

	t.Run("直接调用不经 WithContext：保持原行为、不带 trace.id", func(t *testing.T) {
		l, logs := newObserved()

		l.Info("hi", logger.String("k", "v"))

		fields := logs.All()[0].ContextMap()
		_, hasTrace := fields["trace.id"]
		assert.False(t, hasTrace)
		assert.Equal(t, "v", fields["k"])
	})
}
