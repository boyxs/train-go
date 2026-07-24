package service

import (
	"context"
	"sync"

	commentv1 "github.com/boyxs/train-go/webook/api/gen/comment/v1"
	feedv1 "github.com/boyxs/train-go/webook/api/gen/feed/v1"
	"github.com/boyxs/train-go/webook/internal/domain"
	"github.com/boyxs/train-go/webook/pkg/logger"
)

const (
	feedDefaultLimit = 10
	feedMaxLimit     = 20
)

// FeedService 关注流 core 网关业务：调 feed gRPC 拿文章 id 流，聚合 article(标题/摘要，撤回过滤) +
// user(昵称) + interaction(点赞/收藏) + comment(评论数) + tag(标签) → 卡片。接入层只调用 + VO 映射。
type FeedService interface {
	// List 关注流游标分页：返回卡片、下一页游标、是否还有更多。
	List(ctx context.Context, uid, cursor int64, limit int) ([]domain.FeedArticleItem, int64, bool, error)
	// NewCount 自 since 游标以来的新文章数（P1 提示条轮询）。
	NewCount(ctx context.Context, uid, since int64) (int64, error)
}

type GRPCFeedService struct {
	articleSvc ArticleReaderService
	intrSvc    InteractionService
	commentCli commentv1.CommentServiceClient
	feedClient feedv1.FeedServiceClient
	tagSvc     TagService
	userSvc    UserService
	l          logger.LoggerX
}

func NewGRPCFeedService(
	feedClient feedv1.FeedServiceClient,
	articleSvc ArticleReaderService,
	intrSvc InteractionService,
	commentCli commentv1.CommentServiceClient,
	tagSvc TagService,
	userSvc UserService,
	l logger.LoggerX,
) FeedService {
	return &GRPCFeedService{
		feedClient: feedClient, articleSvc: articleSvc, intrSvc: intrSvc,
		commentCli: commentCli, tagSvc: tagSvc, userSvc: userSvc, l: l,
	}
}

func (s *GRPCFeedService) List(ctx context.Context, uid, cursor int64, limit int) ([]domain.FeedArticleItem, int64, bool, error) {
	if limit <= 0 {
		limit = feedDefaultLimit
	}
	if limit > feedMaxLimit {
		limit = feedMaxLimit
	}
	resp, err := s.feedClient.ListFeed(ctx, &feedv1.ListFeedRequest{Uid: uid, Cursor: cursor, Limit: int32(limit)})
	if err != nil {
		return nil, 0, false, err
	}
	feedItems := resp.GetItems()
	if len(feedItems) == 0 {
		return nil, resp.GetNextCursor(), resp.GetHasMore(), nil
	}

	ids := make([]int64, 0, len(feedItems))
	for _, it := range feedItems {
		ids = append(ids, it.GetArticleId())
	}

	// article 详情是必需源（提供标题/摘要 + 撤回过滤：撤回文章不在 published_article，天然被滤）。
	articles, err := s.articleSvc.BatchDetail(ctx, ids)
	if err != nil {
		return nil, 0, false, err
	}
	artMap := make(map[int64]domain.Article, len(articles))
	authorIds := make([]int64, 0, len(articles))
	seenAuthor := make(map[int64]struct{}, len(articles))
	for _, a := range articles {
		artMap[a.Id] = a
		if _, ok := seenAuthor[a.Author.Id]; !ok {
			seenAuthor[a.Author.Id] = struct{}{}
			authorIds = append(authorIds, a.Author.Id)
		}
	}

	// 4 源相互独立、各自内部降级（失败填零/空、不返错），并发跑：聚合延迟从四者之和降到四者最大值。
	var (
		intrMap map[int64]domain.Interaction
		cmtMap  map[int64]int64
		tagMap  map[int64][]domain.Tag
		userMap map[int64]domain.User
		wg      sync.WaitGroup
	)
	wg.Add(4)
	go func() { defer wg.Done(); intrMap = s.batchInteraction(ctx, ids) }()
	go func() { defer wg.Done(); cmtMap = s.commentCounts(ctx, ids) }()
	go func() { defer wg.Done(); tagMap = s.batchTags(ctx, ids) }()
	go func() { defer wg.Done(); userMap = s.batchUsers(ctx, authorIds) }()
	wg.Wait()

	// 按 feed 顺序构建卡片，撤回/删除的文章跳过（读时过滤兜底）。
	result := make([]domain.FeedArticleItem, 0, len(feedItems))
	for _, it := range feedItems {
		a, ok := artMap[it.GetArticleId()]
		if !ok {
			continue
		}
		intr := intrMap[a.Id]
		result = append(result, domain.FeedArticleItem{
			ArticleId:   a.Id,
			Title:       a.Title,
			Abstract:    a.DisplayAbstract(),
			Author:      domain.Author{Id: a.Author.Id, Name: userMap[a.Author.Id].Nickname},
			PublishedAt: it.GetPublishedAt(),
			LikeCnt:     intr.LikeCount,
			CollectCnt:  intr.CollectCount,
			CommentCnt:  cmtMap[a.Id],
			Tags:        tagMap[a.Id],
		})
	}
	return result, resp.GetNextCursor(), resp.GetHasMore(), nil
}

func (s *GRPCFeedService) NewCount(ctx context.Context, uid, since int64) (int64, error) {
	resp, err := s.feedClient.NewCount(ctx, &feedv1.NewCountRequest{Uid: uid, SinceCursor: since})
	if err != nil {
		return 0, err
	}
	ids := resp.GetArticleIds()
	if len(ids) == 0 {
		return 0, nil
	}
	// feed 收件箱含已撤回/软删文章（撤回是读时过滤、inbox 不摘除）；只数可见（未撤回）的，与列表口径一致，
	// 避免把撤回文章当新文章反复提示。用轻量 COUNT（CountByIds），不为计数捞全字段。
	return s.articleSvc.CountByIds(ctx, ids)
}

// batchInteraction 批量互动计数；失败降级空 map。
func (s *GRPCFeedService) batchInteraction(ctx context.Context, ids []int64) map[int64]domain.Interaction {
	m, err := s.intrSvc.FindByBizIds(ctx, domain.BizArticle, ids)
	if err != nil {
		s.l.Error(ctx, "feed 聚合互动计数失败，降级填零", logger.Error(err))
		return map[int64]domain.Interaction{}
	}
	return m
}

// commentCounts 一次 BatchCountComment 取评论数；失败降级空 map。
func (s *GRPCFeedService) commentCounts(ctx context.Context, ids []int64) map[int64]int64 {
	resp, err := s.commentCli.BatchCountComment(ctx, &commentv1.BatchCountCommentRequest{Biz: domain.BizArticle, BizIds: ids})
	if err != nil {
		s.l.Error(ctx, "feed 聚合评论数失败，降级填零", logger.Error(err))
		return map[int64]int64{}
	}
	return resp.GetCounts()
}

// batchTags 一次 TagsByBiz 取标签；失败降级空 map。
func (s *GRPCFeedService) batchTags(ctx context.Context, ids []int64) map[int64][]domain.Tag {
	m, err := s.tagSvc.TagsByBiz(ctx, domain.BizArticle, ids)
	if err != nil {
		s.l.Error(ctx, "feed 聚合标签失败，降级不带标签", logger.Error(err))
		return map[int64][]domain.Tag{}
	}
	return m
}

// batchUsers 批量取作者昵称；失败降级空 map。
func (s *GRPCFeedService) batchUsers(ctx context.Context, authorIds []int64) map[int64]domain.User {
	if len(authorIds) == 0 {
		return map[int64]domain.User{}
	}
	m, err := s.userSvc.FindByIds(ctx, authorIds)
	if err != nil {
		s.l.Error(ctx, "feed 聚合作者昵称失败，降级空昵称", logger.Error(err))
		return map[int64]domain.User{}
	}
	return m
}
