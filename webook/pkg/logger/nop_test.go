package logger_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/boyxs/train-go/webook/pkg/logger"
)

func TestNopLogger_WithContext(t *testing.T) {
	l := logger.NewNopLogger()
	assert.NotPanics(t, func() {
		l.WithContext(context.Background()).Info("noop", logger.String("k", "v"))
	})
}
