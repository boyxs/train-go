package service

import (
	"context"
	"errors"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/boyxs/train-go/webook/internal/domain"
	repomocks "github.com/boyxs/train-go/webook/internal/repository/mocks"
	svcmocks "github.com/boyxs/train-go/webook/internal/service/mocks"
	"github.com/boyxs/train-go/webook/pkg/logger"
	"github.com/boyxs/train-go/webook/pkg/pool"
)

// testCounter 未注册的一次性计数器，给不关心丢弃指标的用例占位。
func testCounter() prometheus.Counter {
	return prometheus.NewCounter(prometheus.CounterOpts{Name: "test_boost_dropped"})
}

// Like 成功 → 经协程池 boost +3 到当日 hot 榜。pool.Stop 排空后由 ctrl.Finish 校验 IncrScore 被调。
func TestArticleRankingAware_BoostsOnSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	inner := svcmocks.NewMockInteractionService(ctrl)
	repo := repomocks.NewMockRankingRepository(ctrl)
	inner.EXPECT().Like(gomock.Any(), int64(7), domain.BizArticle, int64(100)).Return(nil)
	repo.EXPECT().IncrScore(gomock.Any(), gomock.Any(), string(domain.DimensionHot), "", int64(100), 3.0).Return(nil)

	p := pool.New(2, 16)
	svc := NewArticleRankingAwareInteractionService(inner, repo, p, testCounter(), logger.NewNopLogger())
	require.NoError(t, svc.Like(context.Background(), 7, domain.BizArticle, 100))
	p.Shutdown()
}

// 取消收藏 → 负 delta（-5）；非 article 业务不 boost（无 IncrScore EXPECT，误调即失败）。
func TestArticleRankingAware_CancelNegativeAndNonArticleSkip(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	inner := svcmocks.NewMockInteractionService(ctrl)
	repo := repomocks.NewMockRankingRepository(ctrl)
	inner.EXPECT().CancelCollect(gomock.Any(), int64(7), domain.BizArticle, int64(100)).Return(nil)
	repo.EXPECT().IncrScore(gomock.Any(), gomock.Any(), string(domain.DimensionHot), "", int64(100), -5.0).Return(nil)
	inner.EXPECT().Like(gomock.Any(), int64(7), domain.BizComment, int64(200)).Return(nil)

	p := pool.New(2, 16)
	svc := NewArticleRankingAwareInteractionService(inner, repo, p, testCounter(), logger.NewNopLogger())
	require.NoError(t, svc.CancelCollect(context.Background(), 7, domain.BizArticle, 100))
	require.NoError(t, svc.Like(context.Background(), 7, domain.BizComment, 200))
	p.Shutdown()
}

// 内层失败不 boost（boost 只在成功后触发）。
func TestArticleRankingAware_NoBoostOnInnerError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	inner := svcmocks.NewMockInteractionService(ctrl)
	repo := repomocks.NewMockRankingRepository(ctrl)
	inner.EXPECT().Like(gomock.Any(), int64(7), domain.BizArticle, int64(100)).Return(errors.New("inner down"))

	p := pool.New(2, 16)
	svc := NewArticleRankingAwareInteractionService(inner, repo, p, testCounter(), logger.NewNopLogger())
	require.Error(t, svc.Like(context.Background(), 7, domain.BizArticle, 100))
	p.Shutdown()
}

// 池满时 boost 被丢弃 → boost_dropped 计数 +1，且任务不执行（IncrScore 不被调）。
func TestArticleRankingAware_BoostDroppedMetric(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	inner := svcmocks.NewMockInteractionService(ctrl)
	repo := repomocks.NewMockRankingRepository(ctrl) // 无 IncrScore EXPECT：任务被丢，不该执行
	dropped := prometheus.NewCounter(prometheus.CounterOpts{Name: "test_dropped_metric"})

	started, block := make(chan struct{}), make(chan struct{})
	p := pool.New(1, 1)
	require.True(t, p.Submit(func() { close(started); <-block })) // 占住唯一 worker
	<-started
	require.True(t, p.Submit(func() {})) // 占满队列（容量 1）

	inner.EXPECT().Like(gomock.Any(), int64(7), domain.BizArticle, int64(100)).Return(nil)
	svc := NewArticleRankingAwareInteractionService(inner, repo, p, dropped, logger.NewNopLogger())
	require.NoError(t, svc.Like(context.Background(), 7, domain.BizArticle, 100)) // boost 入队失败被丢

	assert.Equal(t, float64(1), testutil.ToFloat64(dropped))
	close(block)
	p.Shutdown()
}
