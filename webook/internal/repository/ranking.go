package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/golang-module/carbon/v2"
	"golang.org/x/sync/errgroup"

	"github.com/boyxs/train-go/webook/internal/consts"
	"github.com/boyxs/train-go/webook/internal/domain"
	"github.com/boyxs/train-go/webook/internal/repository/cache"
	"github.com/boyxs/train-go/webook/internal/repository/dao"
	"github.com/boyxs/train-go/webook/pkg/logger"
)

const (
	cacheFallbackLookbackDays = 2
	cacheOpTimeout            = 500 * time.Millisecond
	fallbackDAOTimeout        = 2 * time.Second
)

type RankingRepository interface {
	Top(ctx context.Context, date, dim, cat string, limit int) ([]domain.ArticleRanking, error)
	ReplaceTop(ctx context.Context, date, dim, cat string, items []domain.ArticleRanking) error
	IncrScore(ctx context.Context, date, dim, cat string, articleId int64, delta float64) error

	PrevRanks(ctx context.Context, date, dim, cat string, articleIds []int64) (map[int64]int, error)

	Archive(ctx context.Context, date string) error
	ListArchiveDates(ctx context.Context) ([]string, error)
}

type CacheArticleRankingRepository struct {
	cache cache.RankingCache
	dao   dao.RankingDAO
	l     logger.LoggerX
}

func NewCacheArticleRankingRepository(c cache.RankingCache, d dao.RankingDAO, l logger.LoggerX) RankingRepository {
	return &CacheArticleRankingRepository{cache: c, dao: d, l: l}
}

func (r *CacheArticleRankingRepository) Top(ctx context.Context, date, dim, cat string, limit int) ([]domain.ArticleRanking, error) {
	cacheCtx, cancel := context.WithTimeout(ctx, cacheOpTimeout)
	defer cancel()
	baseItems, err := r.cache.Top(cacheCtx, date, dim, cat, limit)
	if err != nil {
		r.l.Error("cache.Top 失败，降级查归档快照",
			logger.String("date", date), logger.String("dim", dim), logger.String("cat", cat),
			logger.Error(err))
		return r.fallbackFromDAO(date, dim, cat, limit)
	}
	if len(baseItems) == 0 {
		return r.fallbackFromDAO(date, dim, cat, limit)
	}
	ids := make([]int64, 0, len(baseItems))
	for _, it := range baseItems {
		ids = append(ids, it.ArticleId)
	}
	// detail 和 prevRank 互不依赖，并发拉取省一个 RTT；失败降级不阻塞主路径
	detailCtx, cancelDetail := context.WithTimeout(ctx, cacheOpTimeout)
	defer cancelDetail()
	prevCtx, cancelPrev := context.WithTimeout(ctx, cacheOpTimeout)
	defer cancelPrev()
	var (
		detailMap map[int64]domain.ArticleRanking
		prevMap   map[int64]int
		eg        errgroup.Group
	)
	eg.Go(func() error {
		m, e := r.cache.GetDetails(detailCtx, date, ids)
		if e != nil {
			r.l.Error("读取榜单 detail 失败", logger.Error(e))
			detailMap = map[int64]domain.ArticleRanking{}
			return nil
		}
		detailMap = m
		return nil
	})
	eg.Go(func() error {
		m, e := r.cache.GetPrevRanks(prevCtx, date, dim, cat, ids)
		if e != nil {
			r.l.Error("读取 prevRank 失败", logger.Error(e))
			prevMap = map[int64]int{}
			return nil
		}
		prevMap = m
		return nil
	})
	// errgroup.Wait 永不返错（两个 goroutine 失败都吞掉返 nil），忽略返回值
	_ = eg.Wait()
	var topScore float64
	if len(baseItems) > 0 {
		topScore = baseItems[0].Score
	}

	items := make([]domain.ArticleRanking, 0, len(baseItems))
	for _, it := range baseItems {
		d, ok := detailMap[it.ArticleId]
		if ok {
			it.Title = d.Title
			it.Author = d.Author
			it.Category = d.Category
			it.Clicks = d.Clicks
			it.Likes = d.Likes
			it.Collects = d.Collects
		}
		if topScore > 0 {
			it.ScoreRatio = it.Score / topScore
		}
		it.Trend, it.TrendDelta = computeTrend(prevMap, it.ArticleId, it.Rank)
		items = append(items, it)
	}
	return items, nil
}

// ReplaceTop 替换当日榜单。关键顺序：先快照旧 rank，再覆盖新榜 —— 否则趋势就没法算。
func (r *CacheArticleRankingRepository) ReplaceTop(ctx context.Context, date, dim, cat string, items []domain.ArticleRanking) error {
	// 第一步：把**当前 Redis 里的旧榜**的 rank 快照到 prevRank Hash
	// 下次 Top() 查 prevRank 对比新 rank 就能算出 ↑↓ 趋势
	currTop, err := r.cache.Top(ctx, date, dim, cat, consts.ArticleRankingTopN)
	if err == nil && len(currTop) > 0 {
		ranks := make(map[int64]int, len(currTop))
		for _, it := range currTop {
			ranks[it.ArticleId] = it.Rank
		}
		if sErr := r.cache.SnapshotRanks(ctx, date, dim, cat, ranks); sErr != nil {
			r.l.Error("快照 prevRank 失败", logger.Error(sErr))
		}
	}

	// 第二步：覆盖 ZSet 为新榜（Del + ZAdd + Expire，用 Pipeline 尽量原子）
	if err := r.cache.ReplaceTop(ctx, date, dim, cat, items); err != nil {
		return err
	}
	// 第三步：写 detail Hash（仅总榜写，分区榜共用同一份 detail 避免重复存储）
	if cat == "" {
		details := make(map[int64]domain.ArticleRanking, len(items))
		for _, it := range items {
			details[it.ArticleId] = it
		}
		if sErr := r.cache.SetDetails(ctx, date, details); sErr != nil {
			// detail 写失败只影响 Top() 读时缺标题/作者，不阻塞榜单本身
			r.l.Error("写 detail 失败", logger.Error(sErr))
		}
	}
	return nil
}

func (r *CacheArticleRankingRepository) IncrScore(ctx context.Context, date, dim, cat string, articleId int64, delta float64) error {
	return r.cache.IncrScore(ctx, date, dim, cat, articleId, delta)
}

func (r *CacheArticleRankingRepository) PrevRanks(ctx context.Context, date, dim, cat string, articleIds []int64) (map[int64]int, error) {
	return r.cache.GetPrevRanks(ctx, date, dim, cat, articleIds)
}

// Archive 归档当日榜单到 DAO，然后清空 Redis。
// 归档维度 = 总榜 3 种（hot/new/best，cat="") + 分区榜 N 个（category × N cat）。
// 任一归档失败整体返回错误，DelDay 不会执行 —— Redis 保留原始数据留待手动排查。
func (r *CacheArticleRankingRepository) Archive(ctx context.Context, date string) error {
	dims := []domain.Dimension{domain.DimensionHot, domain.DimensionNew, domain.DimensionBest}
	for _, d := range dims {
		if err := r.archiveOne(ctx, date, string(d), ""); err != nil {
			return err
		}
	}
	for _, cat := range consts.AllCategories {
		if err := r.archiveOne(ctx, date, string(domain.DimensionCategory), cat); err != nil {
			return err
		}
	}
	// 所有维度归档完才清 Redis，避免半归档状态下 Redis 先被清空
	return r.cache.DelDay(ctx, date)
}

func (r *CacheArticleRankingRepository) archiveOne(ctx context.Context, date, dim, cat string) error {
	items, err := r.cache.Top(ctx, date, dim, cat, consts.ArticleRankingTopN)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return nil
	}
	models := r.toModels(items, cat)
	return r.dao.InsertSnapshot(ctx, date, dim, cat, models)
}

func (r *CacheArticleRankingRepository) ListArchiveDates(ctx context.Context) ([]string, error) {
	return r.dao.ListArchiveDates(ctx)
}

// ── 内部 ─────────────────────────────────────────────────────────────────

// fallbackFromDAO Cache miss 时的降级路径：
//  1. 独立 ctx（不复用请求 ctx）—— 前端断开不影响兜底查询完成
//  2. i=0 先查请求日期本身（可能是用户显式选了某个历史日期），再逐天回溯
//  3. 最多回溯 N 天，都没数据返空切片而不是 err，前端展示占位即可
func (r *CacheArticleRankingRepository) fallbackFromDAO(date, dim, cat string, limit int) ([]domain.ArticleRanking, error) {
	ctx, cancel := context.WithTimeout(context.Background(), fallbackDAOTimeout)
	defer cancel()
	base := carbon.Parse(date)
	if base.IsInvalid() {
		return nil, fmt.Errorf("invalid date %q: %w", date, base.Error)
	}
	for i := 0; i <= cacheFallbackLookbackDays; i++ {
		cur := base.SubDays(i).ToDateString()
		models, err := r.dao.ListByDate(ctx, cur, dim, cat)
		if err != nil {
			r.l.Error("fallback ListByDate 失败", logger.String("date", cur), logger.Error(err))
			continue
		}
		if len(models) == 0 {
			continue
		}
		items := make([]domain.ArticleRanking, 0, len(models))
		for _, m := range models {
			if len(items) >= limit {
				break
			}
			var it domain.ArticleRanking
			if err := json.Unmarshal([]byte(m.Snapshot), &it); err != nil {
				it = domain.ArticleRanking{Rank: m.Rank, ArticleId: m.ArticleId, Score: m.Score}
			}
			// snapshot JSON 里的 trend/trendDelta 是归档时的真实趋势，保留原值
			items = append(items, it)
		}
		return items, nil
	}
	return []domain.ArticleRanking{}, nil
}

func (r *CacheArticleRankingRepository) toModels(items []domain.ArticleRanking, cat string) []dao.ArticleRanking {
	out := make([]dao.ArticleRanking, 0, len(items))
	now := time.Now().UnixMilli()
	for _, it := range items {
		bs, err := json.Marshal(it)
		if err != nil {
			r.l.Error("序列化 rank item 失败", logger.Error(err))
			continue
		}
		out = append(out, dao.ArticleRanking{
			Rank:      it.Rank,
			ArticleId: it.ArticleId,
			Score:     it.Score,
			Snapshot:  string(bs),
			CreatedAt: now,
		})
	}
	return out
}

// computeTrend 算单篇文章的榜单趋势。
// 约定：rank 数字小 = 排名高（1 是第一名），所以 prev - curr > 0 代表排名上升。
// 例：prev=5, curr=2 → delta=3 → TrendUp +3。
func computeTrend(prevMap map[int64]int, articleId int64, currRank int) (domain.Trend, int) {
	prev, ok := prevMap[articleId]
	if !ok {
		return domain.TrendNew, 0
	}
	delta := prev - currRank
	switch {
	case delta > 0:
		return domain.TrendUp, delta
	case delta < 0:
		// delta 是负数，取反返回给前端展示 "↓N"
		return domain.TrendDown, -delta
	default:
		return domain.TrendSame, 0
	}
}
