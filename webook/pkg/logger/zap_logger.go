package logger

import (
	"context"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type ZapLogger struct {
	l *zap.Logger
}

func (z *ZapLogger) Debug(ctx context.Context, msg string, args ...Field) {
	z.l.Debug(msg, z.toArgs(ctx, args)...)
}

func (z *ZapLogger) Info(ctx context.Context, msg string, args ...Field) {
	z.l.Info(msg, z.toArgs(ctx, args)...)
}

func (z *ZapLogger) Warn(ctx context.Context, msg string, args ...Field) {
	z.l.Warn(msg, z.toArgs(ctx, args)...)
}

func (z *ZapLogger) Error(ctx context.Context, msg string, args ...Field) {
	z.l.Error(msg, z.toArgs(ctx, args)...)
}

func (z *ZapLogger) toArgs(ctx context.Context, args []Field) []zap.Field {
	fields := make([]zap.Field, 0, len(args)+2)
	if ctx != nil {
		if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
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
	// AddCallerSkip(1) 跳过本包 Debug/Info/... 包装层，log.origin 指到真正业务调用点而非 zap_logger.go
	return &ZapLogger{
		l: l.WithOptions(zap.AddCallerSkip(1)),
	}
}
