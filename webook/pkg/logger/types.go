package logger

import "context"

type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}

// LoggerX 强制携带 ctx：ctx 用于注入 trace.id/span.id，全链路日志与调用链天然对齐。
// 无请求上下文的场景（ioc/main/后台 goroutine）显式传 context.Background()。
type LoggerX interface {
	Debug(ctx context.Context, msg string, args ...Field)
	Info(ctx context.Context, msg string, args ...Field)
	Warn(ctx context.Context, msg string, args ...Field)
	Error(ctx context.Context, msg string, args ...Field)
}

type Field struct {
	Key string
	Val any
}
