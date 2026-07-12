package service

import (
	"context"
	"sort"

	"golang.org/x/sync/errgroup"

	searchv1 "github.com/boyxs/train-go/webook/api/gen/search/v1"
	tagv1 "github.com/boyxs/train-go/webook/api/gen/tag/v1"
	"github.com/boyxs/train-go/webook/internal/domain"
	"github.com/boyxs/train-go/webook/pkg/logger"
	"github.com/boyxs/train-go/webook/pkg/slicex"
)

const (
	recommendTagsK   = 8   // AI 荐标签返回上限（架构 ≤8）
	tagArticleWindow = 200 // /tag/:slug/articles 候选窗口；core 内存排序分页，深翻页近似（精确热榜 P1）
	tagSortHot       = "hot"
)

// TagService core 侧标签 BFF：发布链路同步（SyncTags/ClearTags）+ typeahead/荐标签/详情/标签下文章聚合。
// 与 GRPCCommentService 同构：持 tag client + 兄弟 search client + article reader / interaction，service 层聚合。
// TagArticles 强耦合 article（联 article reader）。
type TagService interface {
	// SyncTags 把 (biz,bizId) 标签全量对齐到 names，返回已解析标签（含 slug）。
	SyncTags(ctx context.Context, biz string, bizId int64, names []string, source string) ([]domain.Tag, error)
	// ClearTags 清空 (biz,bizId) 关联（= SyncTags 空 names，ref_count 连带 -1）。
	ClearTags(ctx context.Context, biz string, bizId int64) error
	Suggest(ctx context.Context, prefix string, limit int) ([]domain.Tag, error)
	// Recommend kNN 相似文章标签聚合（search）+ 补名字（tag）。
	Recommend(ctx context.Context, title, content string) ([]domain.TagCount, error)
	// Detail 标签详情 + viewer 关注态：Detail(tag 本体，含 followCount) 与 FollowStatus 并发聚合。
	// viewerId<=0（未登录）→ isFollowing 恒 false、跳过 FollowStatus；关注态失败非致命降级 false。
	Detail(ctx context.Context, slug string, viewerId int64) (domain.Tag, bool, error)
	// Follow / Unfollow uid 关注/取关标签，返回是否真翻转（幂等 changed=false）+ 翻转后关注数。
	Follow(ctx context.Context, uid int64, slug string) (changed bool, followerCount int64, err error)
	Unfollow(ctx context.Context, uid int64, slug string) (changed bool, followerCount int64, err error)
	// TagArticles 标签下文章：tag.BizIdsByTag 候选窗口 → article reader 批量 + interaction + tag，内存排序分页。
	TagArticles(ctx context.Context, slug, sortBy string, page, size int) (domain.SearchResult, error)
	// TagsByBiz 批量取多个对象各自的标签（文章详情回显补标签）；返回 bizId → 标签。
	TagsByBiz(ctx context.Context, biz string, bizIds []int64) (map[int64][]domain.Tag, error)
}

type GRPCTagService struct {
	tagCli    tagv1.TagServiceClient
	searchCli searchv1.SearchServiceClient // Recommend 走 search kNN
	readerSvc ArticleReaderService
	intrSvc   InteractionService
	l         logger.LoggerX
}

func NewGRPCTagService(tagCli tagv1.TagServiceClient, searchCli searchv1.SearchServiceClient, readerSvc ArticleReaderService, intrSvc InteractionService, l logger.LoggerX) TagService {
	return &GRPCTagService{tagCli: tagCli, searchCli: searchCli, readerSvc: readerSvc, intrSvc: intrSvc, l: l}
}

func (s *GRPCTagService) SyncTags(ctx context.Context, biz string, bizId int64, names []string, source string) ([]domain.Tag, error) {
	resp, err := s.tagCli.SyncTags(ctx, &tagv1.SyncTagsRequest{Biz: biz, BizId: bizId, Names: names, Source: source})
	if err != nil {
		return nil, err
	}
	return slicex.Map(resp.GetTags(), toDomainTag), nil
}

func (s *GRPCTagService) ClearTags(ctx context.Context, biz string, bizId int64) error {
	_, err := s.tagCli.SyncTags(ctx, &tagv1.SyncTagsRequest{Biz: biz, BizId: bizId, Names: nil, Source: ""})
	return err
}

func (s *GRPCTagService) Suggest(ctx context.Context, prefix string, limit int) ([]domain.Tag, error) {
	resp, err := s.tagCli.Suggest(ctx, &tagv1.SuggestRequest{Prefix: prefix, Limit: int32(limit)})
	if err != nil {
		return nil, err
	}
	return slicex.Map(resp.GetTags(), toDomainTag), nil
}

func (s *GRPCTagService) Recommend(ctx context.Context, title, content string) ([]domain.TagCount, error) {
	resp, err := s.searchCli.RecommendTags(ctx, &searchv1.RecommendTagsRequest{Title: title, Content: content, K: recommendTagsK})
	if err != nil {
		return nil, err
	}
	facets := slicex.Map(resp.GetTags(), func(t *searchv1.TagCount) domain.TagCount {
		return domain.TagCount{Slug: t.GetSlug(), Count: t.GetCount()}
	})
	if len(facets) == 0 {
		return []domain.TagCount{}, nil
	}
	slugs := make([]string, 0, len(facets))
	for _, f := range facets {
		slugs = append(slugs, f.Slug)
	}
	nameMap := s.resolveTagNames(ctx, slugs)
	for i := range facets {
		facets[i].Name = orSlug(nameMap[facets[i].Slug], facets[i].Slug)
	}
	return facets, nil
}

func (s *GRPCTagService) Detail(ctx context.Context, slug string, viewerId int64) (domain.Tag, bool, error) {
	var (
		tag         domain.Tag
		isFollowing bool
		eg          errgroup.Group
	)
	eg.Go(func() error {
		resp, err := s.tagCli.Detail(ctx, &tagv1.DetailRequest{Slug: slug})
		if err != nil {
			return err
		}
		tag = toDomainTag(resp)
		return nil
	})
	if viewerId > 0 {
		eg.Go(func() error {
			resp, err := s.tagCli.FollowStatus(ctx, &tagv1.FollowStatusRequest{Uid: viewerId, Slug: slug})
			if err != nil {
				s.l.Error("查询标签关注态失败，降级 false", logger.Error(err))
				return nil // 关注态非致命：不阻断详情
			}
			isFollowing = resp.GetIsFollowing()
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		return domain.Tag{}, false, err
	}
	return tag, isFollowing, nil
}

func (s *GRPCTagService) Follow(ctx context.Context, uid int64, slug string) (bool, int64, error) {
	resp, err := s.tagCli.Follow(ctx, &tagv1.FollowRequest{Uid: uid, Slug: slug})
	if err != nil {
		return false, 0, err
	}
	return resp.GetChanged(), resp.GetFollowerCount(), nil
}

func (s *GRPCTagService) Unfollow(ctx context.Context, uid int64, slug string) (bool, int64, error) {
	resp, err := s.tagCli.Unfollow(ctx, &tagv1.FollowRequest{Uid: uid, Slug: slug})
	if err != nil {
		return false, 0, err
	}
	return resp.GetChanged(), resp.GetFollowerCount(), nil
}

func (s *GRPCTagService) TagArticles(ctx context.Context, slug, sortBy string, page, size int) (domain.SearchResult, error) {
	resp, err := s.tagCli.BizIdsByTag(ctx, &tagv1.BizIdsByTagRequest{Slug: slug, Biz: domain.BizArticle, Limit: tagArticleWindow})
	if err != nil {
		return domain.SearchResult{}, err
	}
	ids, total := resp.GetIds(), resp.GetTotal()
	if len(ids) == 0 {
		return domain.SearchResult{Articles: []domain.TaggedArticle{}, Total: total}, nil
	}
	// BatchDetail / tagsByBiz / interaction 均只依赖 ids、互不依赖 → 并发省 RTT；仅 BatchDetail 返错传播，其余内部降级。
	var articles []domain.Article
	var tagMap map[int64][]domain.Tag
	var intrMap map[int64]domain.Interaction
	var eg errgroup.Group
	eg.Go(func() error {
		var e error
		articles, e = s.readerSvc.BatchDetail(ctx, ids)
		return e
	})
	eg.Go(func() error {
		tagMap = s.tagsByBiz(ctx, ids)
		return nil
	})
	eg.Go(func() error {
		intrMap = s.batchInteraction(ctx, ids)
		return nil
	})
	if err := eg.Wait(); err != nil {
		return domain.SearchResult{}, err
	}
	tagged := make([]domain.TaggedArticle, 0, len(articles))
	for _, a := range articles {
		intr := intrMap[a.Id]
		tagged = append(tagged, domain.TaggedArticle{
			Id:         a.Id,
			Title:      a.Title,
			Abstract:   a.DisplayAbstract(),
			Author:     a.Author,
			Category:   a.Category,
			CreatedAt:  a.CreatedAt,
			Tags:       tagMap[a.Id],
			ReadCnt:    intr.ReadCount,
			LikeCnt:    intr.LikeCount,
			CollectCnt: intr.CollectCount,
		})
	}
	if sortBy == tagSortHot {
		sortTaggedByHot(tagged)
	}
	// 窗口内内存分页（BatchDetail 已按 ids=created_at DESC 保序，即 new 序）
	return domain.SearchResult{Articles: pageTagged(tagged, page, size), Total: total}, nil
}

// TagsByBiz 批量取多个对象各自的标签（bizId → 标签）；错误由调用方决定降级。
func (s *GRPCTagService) TagsByBiz(ctx context.Context, biz string, bizIds []int64) (map[int64][]domain.Tag, error) {
	if len(bizIds) == 0 {
		return map[int64][]domain.Tag{}, nil
	}
	resp, err := s.tagCli.TagsByBiz(ctx, &tagv1.TagsByBizRequest{Biz: biz, BizIds: bizIds})
	if err != nil {
		return nil, err
	}
	res := make(map[int64][]domain.Tag, len(resp.GetTags()))
	for bizId, list := range resp.GetTags() {
		res[bizId] = slicex.Map(list.GetTags(), toDomainTag)
	}
	return res, nil
}

// tagsByBiz 标签页列表补标签：TagsByBiz 的降级封装（失败返回空 map，不阻断列表）。
func (s *GRPCTagService) tagsByBiz(ctx context.Context, ids []int64) map[int64][]domain.Tag {
	m, err := s.TagsByBiz(ctx, domain.BizArticle, ids)
	if err != nil {
		s.l.Error("标签页：批量取文章标签失败，降级空", logger.Error(err))
		return map[int64][]domain.Tag{}
	}
	return m
}

// batchInteraction 批量取文章互动计数；失败降级空 map（列表主流程不阻断）。
func (s *GRPCTagService) batchInteraction(ctx context.Context, ids []int64) map[int64]domain.Interaction {
	if len(ids) == 0 {
		return map[int64]domain.Interaction{}
	}
	m, err := s.intrSvc.FindByBizIds(ctx, domain.BizArticle, ids)
	if err != nil {
		s.l.Error("批量文章互动计数失败，降级填零", logger.Error(err))
		return map[int64]domain.Interaction{}
	}
	return m
}

// resolveTagNames 一次 BatchBySlugs 解析 slug→name；失败降级空 map（调用方用 slug 占位）。
func (s *GRPCTagService) resolveTagNames(ctx context.Context, slugs []string) map[string]string {
	if len(slugs) == 0 {
		return map[string]string{}
	}
	resp, err := s.tagCli.BatchBySlugs(ctx, &tagv1.BatchBySlugsRequest{Slugs: slugs})
	if err != nil {
		s.l.Error("解析标签名失败，降级用 slug", logger.Error(err))
		return map[string]string{}
	}
	m := make(map[string]string, len(resp.GetTags()))
	for _, t := range resp.GetTags() {
		m[t.GetSlug()] = t.GetName()
	}
	return m
}

// ── tagpb ↔ domain 单一映射点──────────────────────────────────────────────

func toDomainTag(t *tagv1.Tag) domain.Tag {
	return domain.Tag{
		Id:             t.GetId(),
		Name:           t.GetName(),
		Slug:           t.GetSlug(),
		Type:           t.GetType(),
		Description:    t.GetDescription(),
		RefCount:       t.GetRefCount(),
		FollowCount:    t.GetFollowCount(),
		WeeklyNewCount: t.GetWeeklyNewCount(),
	}
}

func sortTaggedByHot(items []domain.TaggedArticle) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].LikeCnt != items[j].LikeCnt {
			return items[i].LikeCnt > items[j].LikeCnt
		}
		return items[i].CreatedAt > items[j].CreatedAt
	})
}

// pageTagged 窗口内内存分页（越界返回空切片）。
func pageTagged(items []domain.TaggedArticle, page, size int) []domain.TaggedArticle {
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 10
	}
	offset := (page - 1) * size
	if offset < 0 || offset >= len(items) { // offset<0 兜底 int 溢出（web 层已 clamp page，双保险）
		return []domain.TaggedArticle{}
	}
	end := offset + size
	if end > len(items) {
		end = len(items)
	}
	return items[offset:end]
}
