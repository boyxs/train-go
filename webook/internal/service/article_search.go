package service

import (
	"context"

	"golang.org/x/sync/errgroup"

	searchv1 "github.com/boyxs/train-go/webook/api/gen/search/v1"
	tagv1 "github.com/boyxs/train-go/webook/api/gen/tag/v1"
	"github.com/boyxs/train-go/webook/internal/domain"
	"github.com/boyxs/train-go/webook/pkg/logger"
	"github.com/boyxs/train-go/webook/pkg/slicex"
)

// ArticleSearchService core 侧文章搜索：索引/移除（发布链路）+ /search/article 聚合（命中 + 互动计数 + 标签名 + facet）。
// 与 GRPCCommentService 同构——持下游 client（+ 兄弟服务 client）在 service 层做聚合，接入层只调用。
// 强耦合 article（下游 search 只搜 article_v1）：耦合放类型名，方法一律裸动词（对齐 ArticleAuthorService.Edit/Publish）。
type ArticleSearchService interface {
	Index(ctx context.Context, article domain.Article) error
	Remove(ctx context.Context, id int64) error
	// Search 命中文章（含互动计数与命名标签）+ 总数 + facet（含名字）。
	Search(ctx context.Context, query string, filterTags []string, page, size int) (domain.SearchResult, error)
}

type GRPCArticleSearchService struct {
	searchCli searchv1.SearchServiceClient
	tagCli    tagv1.TagServiceClient // 解析命中/facet 标签 slug→name（search 只回 slug）
	intrSvc   InteractionService
	l         logger.LoggerX
}

func NewGRPCArticleSearchService(searchCli searchv1.SearchServiceClient, tagCli tagv1.TagServiceClient, intrSvc InteractionService, l logger.LoggerX) ArticleSearchService {
	return &GRPCArticleSearchService{searchCli: searchCli, tagCli: tagCli, intrSvc: intrSvc, l: l}
}

func (s *GRPCArticleSearchService) Index(ctx context.Context, article domain.Article) error {
	_, err := s.searchCli.IndexArticle(ctx, &searchv1.IndexArticleRequest{Doc: toPbArticleDoc(article)})
	return err
}

func (s *GRPCArticleSearchService) Remove(ctx context.Context, id int64) error {
	_, err := s.searchCli.RemoveArticle(ctx, &searchv1.RemoveArticleRequest{Id: id})
	return err
}

func (s *GRPCArticleSearchService) Search(ctx context.Context, query string, filterTags []string, page, size int) (domain.SearchResult, error) {
	resp, err := s.searchCli.SearchArticles(ctx, &searchv1.SearchArticlesRequest{
		Query: query, FilterTags: filterTags, Page: int32(page), Size: int32(size),
	})
	if err != nil {
		return domain.SearchResult{}, err
	}
	articles := slicex.Map(resp.GetArticles(), toDomainArticle) // Article.Tags = 命中标签 slug
	facets := slicex.Map(resp.GetFacets(), func(t *searchv1.TagCount) domain.TagCount {
		return domain.TagCount{Slug: t.GetSlug(), Count: t.GetCount()}
	})
	// 补标签名(tag) 与 互动计数(interaction) 互不依赖 → 并发省一跳 RTT；两者内部各自降级不返错。
	var nameMap map[string]string
	var intrMap map[int64]domain.Interaction
	var eg errgroup.Group
	eg.Go(func() error {
		nameMap = s.resolveTagNames(ctx, collectSearchSlugs(articles, facets))
		return nil
	})
	eg.Go(func() error {
		intrMap = s.batchInteraction(ctx, articleIds(articles))
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
			Tags:       namedTags(a.Tags, nameMap),
			ReadCnt:    intr.ReadCount,
			LikeCnt:    intr.LikeCount,
			CollectCnt: intr.CollectCount,
		})
	}
	for i := range facets {
		facets[i].Name = orSlug(nameMap[facets[i].Slug], facets[i].Slug)
	}
	return domain.SearchResult{Articles: tagged, Total: resp.GetTotal(), Facets: facets}, nil
}

// batchInteraction 批量取文章互动计数；失败降级空 map（列表主流程不阻断）。
func (s *GRPCArticleSearchService) batchInteraction(ctx context.Context, ids []int64) map[int64]domain.Interaction {
	if len(ids) == 0 {
		return map[int64]domain.Interaction{}
	}
	m, err := s.intrSvc.FindByBizIds(ctx, domain.BizArticle, ids)
	if err != nil {
		s.l.WithContext(ctx).Error("批量文章互动计数失败，降级填零", logger.Error(err))
		return map[int64]domain.Interaction{}
	}
	return m
}

// resolveTagNames 一次 BatchBySlugs 解析 slug→name；失败降级空 map（调用方用 slug 占位）。
func (s *GRPCArticleSearchService) resolveTagNames(ctx context.Context, slugs []string) map[string]string {
	if len(slugs) == 0 {
		return map[string]string{}
	}
	resp, err := s.tagCli.BatchBySlugs(ctx, &tagv1.BatchBySlugsRequest{Slugs: slugs})
	if err != nil {
		s.l.WithContext(ctx).Error("解析标签名失败，降级用 slug", logger.Error(err))
		return map[string]string{}
	}
	m := make(map[string]string, len(resp.GetTags()))
	for _, t := range resp.GetTags() {
		m[t.GetSlug()] = t.GetName()
	}
	return m
}

// ── searchpb ↔ domain 单一映射点（Index / Search 各自私有）──────────────────

func toPbArticleDoc(a domain.Article) *searchv1.ArticleDoc {
	return &searchv1.ArticleDoc{
		Id:         a.Id,
		Title:      a.Title,
		Abstract:   a.DisplayAbstract(),
		AuthorId:   a.Author.Id,
		AuthorName: a.Author.Name,
		Status:     uint32(a.Status),
		Category:   a.Category,
		Tags:       a.Tags, // 此处应已是解析后的 slug
		CreatedAt:  a.CreatedAt,
		Content:    a.Content, // 仅供 search 生成 content_vec（与 RecommendTags 同口径 embed）
	}
}

func toDomainArticle(c *searchv1.ArticleCard) domain.Article {
	return domain.Article{
		Id:        c.GetId(),
		Title:     c.GetTitle(),
		Abstract:  c.GetAbstract(),
		Author:    domain.Author{Id: c.GetAuthorId(), Name: c.GetAuthorName()},
		Category:  c.GetCategory(),
		CreatedAt: c.GetCreatedAt(),
		Tags:      c.GetTags(),
	}
}

// namedTags 把 slug 列表按 slug→name map 组装成 []domain.Tag；名字缺失用 slug 占位。
func namedTags(slugs []string, nameMap map[string]string) []domain.Tag {
	tags := make([]domain.Tag, 0, len(slugs))
	for _, sl := range slugs {
		tags = append(tags, domain.Tag{Slug: sl, Name: orSlug(nameMap[sl], sl)})
	}
	return tags
}

func collectSearchSlugs(articles []domain.Article, facets []domain.TagCount) []string {
	set := make(map[string]struct{})
	for _, a := range articles {
		for _, sl := range a.Tags {
			set[sl] = struct{}{}
		}
	}
	for _, f := range facets {
		set[f.Slug] = struct{}{}
	}
	slugs := make([]string, 0, len(set))
	for sl := range set {
		slugs = append(slugs, sl)
	}
	return slugs
}

func articleIds(articles []domain.Article) []int64 {
	ids := make([]int64, 0, len(articles))
	for _, a := range articles {
		ids = append(ids, a.Id)
	}
	return ids
}

// orSlug 标签名缺失时用 slug 占位（facet / 命中标签命名共用）。
func orSlug(name, slug string) string {
	if name != "" {
		return name
	}
	return slug
}
