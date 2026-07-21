package logger

import (
	"context"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type ZapLogger struct {
	l   *zap.Logger
	ctx context.Context // WithContext 绑定；nil 表示未绑定（不注入 trace）
}

func (z *ZapLogger) Debug(msg string, args ...Field) {
	z.l.Debug(msg, z.toArgs(args)...)
}

func (z *ZapLogger) Info(msg string, args ...Field) {
	z.l.Info(msg, z.toArgs(args)...)
}

func (z *ZapLogger) Warn(msg string, args ...Field) {
	z.l.Warn(msg, z.toArgs(args)...)
}

func (z *ZapLogger) Error(msg string, args ...Field) {
	z.l.Error(msg, z.toArgs(args)...)
}

func (z *ZapLogger) WithContext(ctx context.Context) LoggerX {
	return &ZapLogger{l: z.l, ctx: ctx}
}

func (z *ZapLogger) toArgs(args []Field) []zap.Field {
	fields := make([]zap.Field, 0, len(args)+2)
	if z.ctx != nil {
		if sc := trace.SpanContextFromContext(z.ctx); sc.IsValid() {
			fields = append(fields,
				zap.String("trace.id", sc.TraceID().String()),
				zap.String("span.id", sc.SpanID().String()))
		}
	}
	for _, arg := range args {
		fields = append(fields, zap.Any(arg.Key, arg.Val))
	}
	return fields
}

func NewZapLogger(l *zap.Logger) LoggerX {
	return &ZapLogger{
		l: l,
	}
}
