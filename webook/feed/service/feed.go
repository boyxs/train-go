package service

import (
	"context"
	"sort"
	"sync"

	"golang.org/x/sync/errgroup"

	articlev1 "github.com/boyxs/train-go/webook/api/gen/article/v1"
	relationv1 "github.com/boyxs/train-go/webook/api/gen/relation/v1"
	"github.com/boyxs/train-go/webook/feed/domain"
	"github.com/boyxs/train-go/webook/feed/repository"
	"github.com/boyxs/train-go/webook/pkg/logger"
)

const (
	// maxOutboxConcurrency 并发读 outbox 的上限（errgroup）。
	maxOutboxConcurrency = 10
	// feedListMaxLimit 读页 limit 上限（BFF 已夹取 1..20，此为本层防御性上限，防 limit<=0 时
	// go-redis Count=0 退化为「不下发 LIMIT → 全量返回」+ hasMore 死循环）。
	feedListMaxLimit = 100
	// maxFanoutIterations ListFollowers 游标循环的失控上限（下游异常固定游标时兜底，非正常路径）。
	maxFanoutIterations = 10000
)

// Config feed 业务调参（走 yaml feed 段）。
type Config struct {
	BigVThreshold       int64 // 粉丝数 >= 此值不写扩散（大V走拉模式）
	FanoutBatch         int   // ListFollowers 每批（relation 服务端 limit 封顶 50）
	RebuildMaxFollowees int64 // 重建时关注数上限
	OutboxSize          int   // 回源/读 outbox 的条数上限（对齐 outbox cap）
	NewCountMax         int   // 新内容提示条候选 id 上限（够前端显示「N 篇」，超出按满显示）
}

// FeedService 关注流三条路径：写扩散(Fanout)、撤回(Remove)、失效重建(InvalidateInboxes)、读(ListFeed)。
type FeedService interface {
	Fanout(ctx context.Context, art domain.FeedArticle) error
	Remove(ctx context.Context, articleId, authorId int64) error
	InvalidateInboxes(ctx context.Context, uids []int64) error
	ListFeed(ctx context.Context, uid, cursor int64, limit int) ([]domain.FeedItem, int64, bool, error)
	NewCount(ctx context.Context, uid, since int64) ([]int64, error) // P1：since 以来候选文章 id（可见性由 BFF 过滤）
}

type InternalFeedService struct {
	repo           repository.FeedRepository
	relationClient relationv1.RelationServiceClient
	articleClient  articlev1.ArticleReaderServiceClient
	cfg            Config
	l              logger.LoggerX
}

func NewInternalFeedService(
	repo repository.FeedRepository,
	relationClient relationv1.RelationServiceClient,
	articleClient articlev1.ArticleReaderServiceClient,
	cfg Config,
	l logger.LoggerX,
) FeedService {
	return &InternalFeedService{
		repo:           repo,
		relationClient: relationClient,
		articleClient:  articleClient,
		cfg:            cfg,
		l:              l,
	}
}

// Fanout 写扩散：普通作者把文章 ZADD 进每个粉丝收件箱；大 V（粉丝数 >= 阈值）跳过扩散，
// 读时经其 outbox 归并（推拉结合）。outbox「存在才追加」无条件先做（冷则 no-op）。
// 失败一律向上传播 → Kafka 整批重投；ZADD/DEL 幂等，重放安全。
func (s *InternalFeedService) Fanout(ctx context.Context, art domain.FeedArticle) error {
	statsResp, err := s.relationClient.GetStats(ctx, &relationv1.GetStatsRequest{Uid: art.AuthorId})
	if err != nil {
		return err
	}
	item := domain.FeedItem{ArticleId: art.ArticleId, PublishedAt: art.PublishedAt}
	if err = s.repo.AppendOutboxIfExists(ctx, art.AuthorId, item); err != nil {
		return err
	}
	if statsResp.GetStats().GetFollowerCnt() >= s.cfg.BigVThreshold {
		return nil // 大 V：拉模式，不写扩散
	}
	var cursor int64
	for i := 0; i < maxFanoutIterations; i++ {
		resp, err := s.relationClient.ListFollowers(ctx, &relationv1.ListRequest{
			Uid: art.AuthorId, Cursor: cursor, Limit: int32(s.cfg.FanoutBatch),
		})
		if err != nil {
			return err
		}
		edges := resp.GetEdges()
		if len(edges) == 0 {
			break
		}
		uids := make([]int64, 0, len(edges))
		for _, e := range edges {
			uids = append(uids, e.GetFollowerId())
		}
		if err = s.repo.AppendInbox(ctx, uids, item); err != nil {
			return err
		}
		cursor = resp.GetNextCursor()
		if cursor <= 0 {
			break
		}
	}
	return nil
}

// Remove 撤回：inbox 不做摘除（读时 BFF 查线上库天然滤掉已撤回），仅 DEL 作者 outbox 强制回源。
// articleId 仅用于调用方语义/日志；outbox 是整体缓存，删后下次读从源头重填（不含撤回文章）。
func (s *InternalFeedService) Remove(ctx context.Context, articleId, authorId int64) error {
	return s.repo.DelOutbox(ctx, authorId)
}

// InvalidateInboxes 关系变更失效重建：DEL 这些用户的 inbox+bigv+built，下次读全量重建。
func (s *InternalFeedService) InvalidateInboxes(ctx context.Context, uids []int64) error {
	return s.repo.Invalidate(ctx, uids)
}

// ListFeed 读关注流：收件箱未重建则先重建；再归并收件箱（普通作者扩散）与大V发件箱（拉模式）。
func (s *InternalFeedService) ListFeed(ctx context.Context, uid, cursor int64, limit int) ([]domain.FeedItem, int64, bool, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > feedListMaxLimit {
		limit = feedListMaxLimit
	}
	built, err := s.repo.InboxBuilt(ctx, uid)
	if err != nil {
		return nil, 0, false, err
	}
	if !built {
		if err = s.rebuild(ctx, uid); err != nil {
			return nil, 0, false, err
		}
	}
	normal, err := s.repo.ReadInbox(ctx, uid, cursor, limit)
	if err != nil {
		return nil, 0, false, err
	}
	bigvs, err := s.repo.ReadBigv(ctx, uid)
	if err != nil {
		return nil, 0, false, err
	}
	outItems, err := s.gatherOutboxes(ctx, bigvs, cursor, limit, "读大V outbox")
	if err != nil {
		return nil, 0, false, err
	}
	merged := mergeFeedItems(normal, outItems, limit)
	var nextCursor int64
	if len(merged) > 0 {
		nextCursor = merged[len(merged)-1].PublishedAt
	}
	return merged, nextCursor, len(merged) == limit, nil
}

// outboxRead 读某作者 outbox：首页(cursor<=0)读到空视为冷缓存 → 回源 core.ListAuthorArticles 填全量再读；
// 翻页(cursor>0)信任缓存（首页已暖）。
func (s *InternalFeedService) outboxRead(ctx context.Context, authorId, cursor int64, limit int) ([]domain.FeedItem, error) {
	items, err := s.repo.ReadOutbox(ctx, authorId, cursor, limit)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 && cursor <= 0 {
		resp, err := s.articleClient.ListAuthorArticles(ctx, &articlev1.ListAuthorArticlesRequest{
			AuthorId: authorId, Limit: int32(s.cfg.OutboxSize),
		})
		if err != nil {
			return nil, err
		}
		fresh := make([]domain.FeedItem, 0, len(resp.GetItems()))
		for _, b := range resp.GetItems() {
			fresh = append(fresh, domain.FeedItem{ArticleId: b.GetId(), PublishedAt: b.GetPublishedAt()})
		}
		if err = s.repo.FillOutbox(ctx, authorId, fresh); err != nil {
			return nil, err
		}
		items, err = s.repo.ReadOutbox(ctx, authorId, cursor, limit)
		if err != nil {
			return nil, err
		}
	}
	return items, nil
}

// gatherOutboxes 并发读多个作者的 outbox（errgroup ≤10）；单作者失败降级跳过并记日志，不影响整体。
func (s *InternalFeedService) gatherOutboxes(ctx context.Context, authorIds []int64, cursor int64, limit int, scene string) ([]domain.FeedItem, error) {
	if len(authorIds) == 0 {
		return nil, nil
	}
	var mu sync.Mutex
	all := make([]domain.FeedItem, 0, len(authorIds)*limit)
	eg := &errgroup.Group{}
	eg.SetLimit(maxOutboxConcurrency)
	for _, aid := range authorIds {
		aid := aid
		eg.Go(func() error {
			items, err := s.outboxRead(ctx, aid, cursor, limit)
			if err != nil {
				s.l.Error(ctx, scene+"失败，降级跳过", logger.Int64("authorId", aid), logger.Error(err))
				return nil // 降级：跳过该作者
			}
			mu.Lock()
			all = append(all, items...)
			mu.Unlock()
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		return nil, err
	}
	return all, nil
}

// mergeFeedItems 归并两路条目：按 articleId 去重，按 (publishedAt DESC, articleId DESC) 排序，截断到 limit。
func mergeFeedItems(a, b []domain.FeedItem, limit int) []domain.FeedItem {
	all := make([]domain.FeedItem, 0, len(a)+len(b))
	all = append(all, a...)
	all = append(all, b...)
	seen := make(map[int64]struct{}, len(all))
	merged := make([]domain.FeedItem, 0, len(all))
	for _, it := range all {
		if _, ok := seen[it.ArticleId]; ok {
			continue
		}
		seen[it.ArticleId] = struct{}{}
		merged = append(merged, it)
	}
	sort.Slice(merged, func(i, j int) bool {
		if merged[i].PublishedAt != merged[j].PublishedAt {
			return merged[i].PublishedAt > merged[j].PublishedAt
		}
		return merged[i].ArticleId > merged[j].ArticleId
	})
	if len(merged) > limit {
		merged = merged[:limit]
	}
	return merged
}

// rebuild 全量重建收件箱：拉关注列表 → 按粉丝数二分大V/普通 → 普通作者文章回源归并进 inbox +
// 大V集存进 bigv（读时归并其 outbox）。并发 rebuild 同 uid 结果幂等、后写覆盖，故不加锁。
func (s *InternalFeedService) rebuild(ctx context.Context, uid int64) error {
	followees, err := s.listFollowees(ctx, uid)
	if err != nil {
		return err
	}
	if len(followees) == 0 {
		return s.repo.SaveInbox(ctx, uid, nil, nil) // 空关注也置 built，避免每次读都重建
	}
	statsMap, err := s.batchStats(ctx, followees)
	if err != nil {
		return err
	}
	var bigvs, normals []int64
	for _, f := range followees {
		if statsMap[f] >= s.cfg.BigVThreshold {
			bigvs = append(bigvs, f)
		} else {
			normals = append(normals, f)
		}
	}
	items, err := s.gatherOutboxes(ctx, normals, 0, s.cfg.OutboxSize, "rebuild 回源作者 outbox")
	if err != nil {
		return err
	}
	// cache SaveInbox 内部 ZADD 归并后裁剪到 inbox_cap（top 2000），此处无需预排序/预裁。
	return s.repo.SaveInbox(ctx, uid, items, bigvs)
}

// listFollowees 游标循环拉关注列表，上限 RebuildMaxFollowees。
func (s *InternalFeedService) listFollowees(ctx context.Context, uid int64) ([]int64, error) {
	followees := make([]int64, 0)
	var cursor int64
	for int64(len(followees)) < s.cfg.RebuildMaxFollowees {
		resp, err := s.relationClient.ListFollowees(ctx, &relationv1.ListRequest{
			Uid: uid, Cursor: cursor, Limit: int32(s.cfg.FanoutBatch),
		})
		if err != nil {
			return nil, err
		}
		edges := resp.GetEdges()
		if len(edges) == 0 {
			break
		}
		for _, e := range edges {
			followees = append(followees, e.GetFolloweeId())
		}
		cursor = resp.GetNextCursor()
		if cursor <= 0 {
			break
		}
	}
	if int64(len(followees)) > s.cfg.RebuildMaxFollowees {
		followees = followees[:s.cfg.RebuildMaxFollowees]
	}
	return followees, nil
}

// batchStats 分批（100）查粉丝数；无记录的 uid 在结果 map 中缺省，查得零值 → 归为普通作者。
func (s *InternalFeedService) batchStats(ctx context.Context, uids []int64) (map[int64]int64, error) {
	const batch = 100
	out := make(map[int64]int64, len(uids))
	for i := 0; i < len(uids); i += batch {
		end := i + batch
		if end > len(uids) {
			end = len(uids)
		}
		resp, err := s.relationClient.BatchGetStats(ctx, &relationv1.BatchGetStatsRequest{Uids: uids[i:end]})
		if err != nil {
			return nil, err
		}
		for k, v := range resp.GetStats() {
			out[k] = v.GetFollowerCnt()
		}
	}
	return out, nil
}

// NewCount 返回自 since 游标以来收件箱中的候选文章 id（DESC，最多 newCountMaxCandidates 条）。
// 只给候选 id、不给最终数：撤回文章按设计仍赖在收件箱（读时过滤，inbox 不摘除），若在此直接计数会把
// 已撤回的算成新文章。可见性（撤回/软删）交给 core BFF 用 BatchDetail 过滤，与列表口径一致。
// 仅收件箱扩散项——大 V outbox 新文不计；收件箱未重建时返回空（用户尚无 since 基准，首次读后即有）。
func (s *InternalFeedService) NewCount(ctx context.Context, uid, since int64) ([]int64, error) {
	built, err := s.repo.InboxBuilt(ctx, uid)
	if err != nil {
		return nil, err
	}
	if !built {
		return nil, nil
	}
	return s.repo.InboxSince(ctx, uid, since, s.cfg.NewCountMax)
}
