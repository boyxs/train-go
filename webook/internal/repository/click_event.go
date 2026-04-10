package repository

import (
	"context"
	"time"

	"gitee.com/train-cloud/geektime-basic-go/internal/domain"
	"gitee.com/train-cloud/geektime-basic-go/internal/repository/cache"
	"gitee.com/train-cloud/geektime-basic-go/internal/repository/dao"
	"gitee.com/train-cloud/geektime-basic-go/pkg/logger"
)

type ClickEventRepository interface {
	RecordClick(ctx context.Context, click domain.ClickEvent) error
	Dashboard(ctx context.Context) (domain.ClickEventDashboard, error)
}

type CacheAIClickEventRepository struct {
	dao   dao.ClickEventDAO
	cache cache.ClickEventCache
	l     logger.LoggerX
}

func NewCacheAIClickEventRepository(d dao.ClickEventDAO, c cache.ClickEventCache, l logger.LoggerX) ClickEventRepository {
	return &CacheAIClickEventRepository{dao: d, cache: c, l: l}
}

func (r *CacheAIClickEventRepository) RecordClick(ctx context.Context, click domain.ClickEvent) error {
	err := r.dao.Insert(ctx, dao.ClickEvent{
		UserId:         click.UserId,
		ArticleId:      click.ArticleId,
		ConversationId: click.ConversationId,
		Source:         click.Source,
	})
	if err != nil {
		return err
	}
	// 写入后清缓存
	if cErr := r.cache.DelDashboard(ctx); cErr != nil {
		r.l.Error("清除看板缓存失败", logger.Error(cErr))
	}
	return nil
}

func (r *CacheAIClickEventRepository) Dashboard(ctx context.Context) (domain.ClickEventDashboard, error) {
	// 查缓存
	data, err := r.cache.GetDashboard(ctx)
	if err == nil {
		return data, nil
	}

	// miss → 查 DAO
	startMs := time.Now().AddDate(0, 0, -30).UnixMilli()
	stats, trends, tops, err := r.dao.Dashboard(ctx, startMs, "ai_chat")
	if err != nil {
		return domain.ClickEventDashboard{}, err
	}

	// 组装 domain
	dashboard := r.toDomain(stats, trends, tops)

	// 回填缓存（失败不阻塞）
	if cErr := r.cache.SetDashboard(ctx, dashboard); cErr != nil {
		r.l.Error("回填看板缓存失败", logger.Error(cErr))
	}

	return dashboard, nil
}

func (r *CacheAIClickEventRepository) toDomain(stats dao.ClickEventStats, trends []dao.ClickEventDailyTrend, tops []dao.ClickEventTopArticle) domain.ClickEventDashboard {
	dailyTrend := make([]domain.DailyTrend, 0, len(trends))
	for _, t := range trends {
		dailyTrend = append(dailyTrend, domain.DailyTrend{Date: t.Date, Clicks: t.Clicks})
	}
	topArticles := make([]domain.TopArticle, 0, len(tops))
	for i, a := range tops {
		topArticles = append(topArticles, domain.TopArticle{
			Rank:        i + 1,
			ArticleId:   a.ArticleId,
			Title:       a.Title,
			Clicks:      a.Clicks,
			UniqueUsers: a.UniqueUsers,
		})
	}

	var avg float64
	if stats.UniqueUsers > 0 {
		avg = float64(stats.TotalClicks) / float64(stats.UniqueUsers)
	}

	return domain.ClickEventDashboard{
		TotalClicks:      stats.TotalClicks,
		UniqueUsers:      stats.UniqueUsers,
		UniqueArticles:   stats.UniqueArticles,
		AvgClicksPerUser: avg,
		DailyTrend:       dailyTrend,
		TopArticles:      topArticles,
	}
}
