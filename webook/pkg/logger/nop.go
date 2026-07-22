package logger

import "context"

type NopLogger struct {
}

func NewNopLogger() LoggerX {
	return &NopLogger{}

}

func (n *NopLogger) Debug(ctx context.Context, msg string, args ...Field) {
}

func (n *NopLogger) Info(ctx context.Context, msg string, args ...Field) {
}

func (n *NopLogger) Warn(ctx context.Context, msg string, args ...Field) {
}

func (n *NopLogger) Error(ctx context.Context, msg string, args ...Field) {
}
