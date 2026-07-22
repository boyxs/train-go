package logger_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/boyxs/train-go/webook/pkg/logger"
)

func TestNopLogger_Ctx(t *testing.T) {
	l := logger.NewNopLogger()
	assert.NotPanics(t, func() {
		l.Info(context.Background(), "noop", logger.String("k", "v"))
	})
}
