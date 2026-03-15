package logger

import "go.uber.org/zap"

type ZapLogger struct {
	l *zap.Logger
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

func (z *ZapLogger) toArgs(args []Field) []zap.Field {
	fields := make([]zap.Field, 0, len(args))
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
