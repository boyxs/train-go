package job

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	svcmocks "github.com/webook/internal/service/mocks"
	"github.com/webook/pkg/logger"
)

// wrap 封装的函数必须：1) 吞 panic 不让 cron goroutine 崩 2) 业务错误只 log 不 panic
func TestRankingJob_wrap_RecoversPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc := svcmocks.NewMockRankingService(ctrl)

	j := NewRankingJob(svc, logger.NewNopLogger())
	called := false
	cb := j.wrap("test_panic", time.Second, func(ctx context.Context, date string) error {
		called = true
		panic("boom")
	})

	assert.NotPanics(t, cb, "cron callback 必须吞掉业务 panic")
	assert.True(t, called, "业务函数应被调到")
}

func TestRankingJob_wrap_SwallowsBusinessError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc := svcmocks.NewMockRankingService(ctrl)

	j := NewRankingJob(svc, logger.NewNopLogger())
	cb := j.wrap("test_err", time.Second, func(ctx context.Context, date string) error {
		return errors.New("svc 报错")
	})
	assert.NotPanics(t, cb, "业务返 error 时 cron callback 不应 panic")
}

func TestRankingJob_wrap_PassesDateString(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc := svcmocks.NewMockRankingService(ctrl)

	j := NewRankingJob(svc, logger.NewNopLogger())
	var gotDate string
	cb := j.wrap("test_date", time.Second, func(ctx context.Context, date string) error {
		gotDate = date
		return nil
	})
	cb()

	// 期望是 YYYY-MM-DD 格式，长度 10
	assert.Len(t, gotDate, 10, "date 必须是 YYYY-MM-DD 形式")
	assert.Regexp(t, `^\d{4}-\d{2}-\d{2}$`, gotDate)
}

func TestRankingJob_RegisterTo_AddsFourEntries(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc := svcmocks.NewMockRankingService(ctrl)

	j := NewRankingJob(svc, logger.NewNopLogger())
	c := cron.New(cron.WithSeconds())
	err := j.RegisterTo(c)
	assert.NoError(t, err)
	// hot / best / new / archive 共 4 个
	assert.Len(t, c.Entries(), 4)
}
