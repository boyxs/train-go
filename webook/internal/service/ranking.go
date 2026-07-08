package service

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/golang-module/carbon/v2"
	"golang.org/x/sync/errgroup"

	"github.com/boyxs/train-go/webook/internal/consts"
	"github.com/boyxs/train-go/webook/internal/domain"
	"github.com/boyxs/train-go/webook/internal/repository"
	"github.com/boyxs/train-go/webook/pkg/logger"
)

const (
	rankingCandidates = 2000           // 候选文章池上限
	bestMinClicks     = 50             // 最佳榜门槛
	newMinClicks      = 10             // 最新榜门槛
	newPublishWindow  = 24 * time.Hour // 最新榜只看当日
)

type RankingService interface {
	Page(ctx context.Context, date, dim, cat string, page, pageSize int) ([]domain.ArticleRanking, int, error)
	RecomputeHot(ctx context.Context, date string) error
	RecomputeBest(ctx context.Context, date string) error
	RecomputeNew(ctx context.Context, date string) error
	Archive(ctx context.Context, date string) error
	ListArchiveDates(ctx context.Context) ([]string, error)
	OnClick(ctx context.Context, uid, articleId int64, rank int, dim string) error
}

type ArticleRankingService struct {
	repo       repository.RankingRepository
	artRepo    repository.ArticleReaderRepository
	interSvc   InteractionService // 经网关读远端 interaction 计数（core 无互动业务逻辑）
	clickEvent ClickEventService
	l          logger.LoggerX
	now        func() time.Time // 可注入 clock，便于测试
}

func NewArticleRankingService(
	repo repository.RankingRepository,
	artRepo repository.ArticleReaderRepository,
	interSvc InteractionService,
	clickEvent ClickEventService,
	l logger.LoggerX,
) RankingService {
	return &ArticleRankingService{
		repo: repo, artRepo: artRepo, interSvc: interSvc,
		clickEvent: clickEvent, l: l,
		now: time.Now,
	}
}

func (s *ArticleRankingService) Page(ctx context.Context, date, dim, cat string, page, pageSize int) ([]domain.ArticleRanking, int, error) {
	if date == "" {
		date = carbon.CreateFromStdTime(s.now()).ToDateString()
	}
	if dim == "" {
		dim = string(domain.DimensionHot)
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	all, err := s.repo.Top(ctx, date, dim, cat, consts.ArticleRankingTopN)
	if err != nil {
		return nil, 0, err
	}
	total := len(all)
	start := (page - 1) * pageSize
	if start >= total {
		return []domain.ArticleRanking{}, total, nil
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	return all[start:end], total, nil
}

// RecomputeHot 重算当日热度榜（总榜 + 5 个分区榜）。
// 分布式锁由 job 层统一处理（pkg/redislockx），service 不感知部署形态。
func (s *ArticleRankingService) RecomputeHot(ctx context.Context, date string) error {
	candidates, err := s.loadCandidates(ctx)
	if err != nil {
		return err
	}
	now := s.now()
	for i := range candidates {
		c := &candidates[i]
		hours := float64(now.UnixMilli()-c.article.CreatedAt) / float64(time.Hour.Milliseconds())
		c.score = HotScore(c.inter.ReadCount, c.inter.LikeCount, c.inter.CollectCount, hours)
	}
	topItems := s.sortAndTake(candidates, consts.ArticleRankingTopN)
	if err := s.repo.ReplaceTop(ctx, date, string(domain.DimensionHot), "", topItems); err != nil {
		return err
	}
	// 分区榜与总榜共享 candidates 和 score，只是按 category 过滤后再排名。
	// 5 个分区彼此独立、写不同 Redis key，并发 fan-out 省 RTT；
	// 单分区失败只记日志不阻塞其他分区（eg.Go 返 nil），Wait 不会返错
	var eg errgroup.Group
	for _, cat := range consts.AllCategories {
		cat := cat
		eg.Go(func() error {
			grouped := s.filterCategory(candidates, cat)
			catTop := s.sortAndTake(grouped, consts.ArticleRankingTopN)
			if err := s.repo.ReplaceTop(ctx, date, string(domain.DimensionCategory), cat, catTop); err != nil {
				s.l.Error("分区榜替换失败", logger.String("cat", cat), logger.Error(err))
			}
			return nil
		})
	}
	_ = eg.Wait()
	return nil
}

func (s *ArticleRankingService) RecomputeBest(ctx context.Context, date string) error {
	candidates, err := s.loadCandidates(ctx)
	if err != nil {
		return err
	}
	out := make([]candidate, 0, len(candidates))
	for _, c := range candidates {
		// bestMinClicks=50 门槛：Wilson 对小样本本身有压制，但 n<50 时方差过大，
		// 误把"1 点击 1 赞"的文章冲到榜首，业务上不想看到
		if c.inter.ReadCount < bestMinClicks {
			continue
		}
		// positives = 赞 + 收藏（两个正向信号累加作为 "好评数"）
		c.score = WilsonLowerBound(c.inter.LikeCount+c.inter.CollectCount, c.inter.ReadCount)
		out = append(out, c)
	}
	topItems := s.sortAndTake(out, consts.ArticleRankingTopN)
	return s.repo.ReplaceTop(ctx, date, string(domain.DimensionBest), "", topItems)
}

func (s *ArticleRankingService) RecomputeNew(ctx context.Context, date string) error {
	candidates, err := s.loadCandidates(ctx)
	if err != nil {
		return err
	}
	// 最新榜只收 24h 内发布的文章，且有最低互动门槛防零互动文章刷屏
	cutoff := s.now().Add(-newPublishWindow).UnixMilli()
	out := make([]candidate, 0, len(candidates))
	for _, c := range candidates {
		if c.article.CreatedAt < cutoff {
			continue
		}
		if c.inter.ReadCount < newMinClicks {
			continue
		}
		// score 直接用发布毫秒戳：越新 → 越大 → 排序越前
		c.score = float64(c.article.CreatedAt)
		out = append(out, c)
	}
	topItems := s.sortAndTake(out, consts.ArticleRankingTopN)
	return s.repo.ReplaceTop(ctx, date, string(domain.DimensionNew), "", topItems)
}

func (s *ArticleRankingService) Archive(ctx context.Context, date string) error {
	return s.repo.Archive(ctx, date)
}

func (s *ArticleRankingService) ListArchiveDates(ctx context.Context) ([]string, error) {
	return s.repo.ListArchiveDates(ctx)
}

// OnClick 记录榜单点击。把 dim 和 rank 编码进 source 字段，避免改 ClickEvent 表结构。
// source 形如 "ranking:hot:3"，dashboard 按 source LIKE 'ranking:%' 聚合即可。
func (s *ArticleRankingService) OnClick(ctx context.Context, uid, articleId int64, rank int, dim string) error {
	if dim == "" {
		dim = string(domain.DimensionUnknown)
	}
	source := fmt.Sprintf(consts.ClickSourceRankingFormat, dim, rank)
	return s.clickEvent.RecordClick(ctx, uid, articleId, 0, source)
}

// ── 内部 ─────────────────────────────────────────────────────────────────

type candidate struct {
	article domain.Article
	inter   domain.Interaction
	score   float64
}

func (s *ArticleRankingService) loadCandidates(ctx context.Context) ([]candidate, error) {
	articleList, _, err := s.artRepo.Page(ctx, 0, rankingCandidates)
	if err != nil {
		return nil, err
	}
	if len(articleList) == 0 {
		return nil, nil
	}
	bizIds := make([]int64, 0, len(articleList))
	for _, a := range articleList {
		bizIds = append(bizIds, a.Id)
	}
	interMap, err := s.interSvc.FindByBizIds(ctx, domain.BizArticle, bizIds)
	if err != nil {
		return nil, err
	}
	out := make([]candidate, 0, len(articleList))
	for _, a := range articleList {
		out = append(out, candidate{article: a, inter: interMap[a.Id]})
	}
	return out, nil
}

func (s *ArticleRankingService) filterCategory(cs []candidate, cat string) []candidate {
	out := make([]candidate, 0, len(cs))
	for _, c := range cs {
		if consts.NormalizeCategory(c.article.Category) == cat {
			out = append(out, c)
		}
	}
	return out
}

func (s *ArticleRankingService) sortAndTake(cs []candidate, n int) []domain.ArticleRanking {
	sort.SliceStable(cs, func(i, j int) bool {
		return cs[i].score > cs[j].score
	})
	if len(cs) > n {
		cs = cs[:n]
	}
	items := make([]domain.ArticleRanking, 0, len(cs))
	for i, c := range cs {
		items = append(items, domain.ArticleRanking{
			Rank:      i + 1,
			ArticleId: c.article.Id,
			Title:     c.article.Title,
			Author:    c.article.Author,
			Category:  consts.NormalizeCategory(c.article.Category),
			Clicks:    c.inter.ReadCount,
			Likes:     c.inter.LikeCount,
			Collects:  c.inter.CollectCount,
			Score:     c.score,
		})
	}
	return items
}
