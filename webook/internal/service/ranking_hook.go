package service

import (
	"context"
	"time"

	"github.com/golang-module/carbon/v2"

	"github.com/webook/internal/domain"
	"github.com/webook/internal/repository"
	"github.com/webook/pkg/logger"
)

// 增量 boost 权重：与 hot 公式 α/β/γ 对齐
const (
	rankingBoostRead    = 1.0
	rankingBoostLike    = 3.0
	rankingBoostCollect = 5.0
	// 同时执行的 boost goroutine 上限。溢出直接丢弃，cron 全量重算兜底。
	rankingBoostConcurrency = 256
)

// ArticleRankingAwareInteractionService 装饰 InteractionService，
// 在 Like / Collect / IncrReadCount 成功后异步 ZINCRBY 当日热度榜。
// 注意是"只读扩展"：互动主流程（DB / Kafka）由 InteractionService 完成，
// 这里追加的 ZINCRBY 失败不回滚主流程 —— cron 全量重算会兜底保证最终一致性。
type ArticleRankingAwareInteractionService struct {
	InteractionService
	rankRepo repository.RankingRepository
	l        logger.LoggerX
	// 信号量：buffered channel 控制并发 goroutine 上限，防止 Redis 卡顿时堆积 OOM
	sem chan struct{}
}

func NewArticleRankingAwareInteractionService(
	base InteractionService,
	rankRepo repository.RankingRepository,
	l logger.LoggerX,
) InteractionService {
	return &ArticleRankingAwareInteractionService{
		InteractionService: base,
		rankRepo:           rankRepo,
		l:                  l,
		sem:                make(chan struct{}, rankingBoostConcurrency),
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

// boost 异步给当日 hot 榜加分。失败只记日志，不影响主流程。
// 通过 buffered channel 当信号量用：sem <- struct{}{} 非阻塞抢占，满了走 default 丢弃，
// goroutine 结束再 <-s.sem 释放。这是 Go 里最轻量的并发限流（不引入 semaphore 库）。
func (s *ArticleRankingAwareInteractionService) boost(biz string, bizId int64, delta float64) {
	// CancelLike / CancelCollect 也会走到这里（传负 delta）；非 article 业务直接跳过
	if biz != domain.BizArticle {
		return
	}
	select {
	case s.sem <- struct{}{}:
		// 抢到信号量槽位，继续启 goroutine
	default:
		// 256 个 goroutine 已在跑，当前这次直接放弃（cron 兜底）
		s.l.Warn("ranking boost 信号量已满，丢弃本次增量",
			logger.Int64("bizId", bizId))
		return
	}
	go func() {
		defer func() { <-s.sem }()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		date := carbon.Now().ToDateString()
		err := s.rankRepo.IncrScore(ctx, date, string(domain.DimensionHot), "", bizId, delta)
		if err != nil {
			s.l.Error("ranking boost 失败",
				logger.Int64("bizId", bizId),
				logger.String("date", date),
				logger.Error(err))
		}
	}()
}
