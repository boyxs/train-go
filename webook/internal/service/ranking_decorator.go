package service

import (
	"context"
	"time"

	"github.com/golang-module/carbon/v2"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/boyxs/train-go/webook/internal/domain"
	"github.com/boyxs/train-go/webook/internal/repository"
	"github.com/boyxs/train-go/webook/pkg/logger"
	"github.com/boyxs/train-go/webook/pkg/pool"
)

// 增量 boost 权重：与 hot 公式 α/β/γ 对齐。
const (
	rankingBoostRead    = 1.0
	rankingBoostLike    = 3.0
	rankingBoostCollect = 5.0
)

// ArticleRankingAwareInteractionService 装饰 InteractionService，
// 在 Like / Collect / IncrReadCount 成功后，经协程池异步 ZINCRBY 当日热度榜。
// 只读扩展：互动主流程（DB / Kafka）由内层完成；boost 失败/丢弃不回滚主流程 —— cron 全量重算兜底。
type ArticleRankingAwareInteractionService struct {
	InteractionService
	rankRepo     repository.RankingRepository
	pool         *pool.Pool
	boostDropped prometheus.Counter
	l            logger.LoggerX
}

func NewArticleRankingAwareInteractionService(
	base InteractionService,
	rankRepo repository.RankingRepository,
	p *pool.Pool,
	boostDropped prometheus.Counter,
	l logger.LoggerX,
) InteractionService {
	return &ArticleRankingAwareInteractionService{
		InteractionService: base,
		rankRepo:           rankRepo,
		pool:               p,
		boostDropped:       boostDropped,
		l:                  l,
	}
}

func (s *ArticleRankingAwareInteractionService) IncrReadCount(ctx context.Context, biz string, bizId int64) error {
	err := s.InteractionService.IncrReadCount(ctx, biz, bizId)
	if err == nil {
		s.boost(biz, bizId, rankingBoostRead)
	}
	return err
}

func (s *ArticleRankingAwareInteractionService) Like(ctx context.Context, uid int64, biz string, bizId int64) error {
	err := s.InteractionService.Like(ctx, uid, biz, bizId)
	if err == nil {
		s.boost(biz, bizId, rankingBoostLike)
	}
	return err
}

func (s *ArticleRankingAwareInteractionService) CancelLike(ctx context.Context, uid int64, biz string, bizId int64) error {
	err := s.InteractionService.CancelLike(ctx, uid, biz, bizId)
	if err == nil {
		s.boost(biz, bizId, -rankingBoostLike)
	}
	return err
}

func (s *ArticleRankingAwareInteractionService) Collect(ctx context.Context, uid int64, biz string, bizId int64) error {
	err := s.InteractionService.Collect(ctx, uid, biz, bizId)
	if err == nil {
		s.boost(biz, bizId, rankingBoostCollect)
	}
	return err
}

func (s *ArticleRankingAwareInteractionService) CancelCollect(ctx context.Context, uid int64, biz string, bizId int64) error {
	err := s.InteractionService.CancelCollect(ctx, uid, biz, bizId)
	if err == nil {
		s.boost(biz, bizId, -rankingBoostCollect)
	}
	return err
}

// boost 经协程池异步给当日 hot 榜加分；队列满则丢弃（cron 全量重算兜底），不阻塞请求。
// 非 article 业务直接跳过。CancelLike / CancelCollect 走到这里传负 delta。
func (s *ArticleRankingAwareInteractionService) boost(biz string, bizId int64, delta float64) {
	if biz != domain.BizArticle {
		return
	}
	ok := s.pool.Submit(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		date := carbon.Now().ToDateString()
		if err := s.rankRepo.IncrScore(ctx, date, string(domain.DimensionHot), "", bizId, delta); err != nil {
			s.l.Error("ranking boost 失败",
				logger.Int64("bizId", bizId), logger.String("date", date), logger.Error(err))
		}
	})
	if !ok {
		s.boostDropped.Inc()
		s.l.Warn("ranking boost 队列已满，丢弃本次增量", logger.Int64("bizId", bizId))
	}
}
