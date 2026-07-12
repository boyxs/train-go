package grpc

import (
	"context"

	searchv1 "github.com/boyxs/train-go/webook/api/gen/search/v1"
	"github.com/boyxs/train-go/webook/pkg/slicex"
	"github.com/boyxs/train-go/webook/search/domain"
)

func (s *SearchServer) SearchArticles(ctx context.Context, req *searchv1.SearchArticlesRequest) (*searchv1.SearchArticlesResponse, error) {
	res, err := s.articleSvc.Search(ctx, req.GetQuery(), req.GetFilterTags(), int(req.GetPage()), int(req.GetSize()))
	if err != nil {
		return nil, err
	}
	return &searchv1.SearchArticlesResponse{
		Articles: slicex.Map(res.Articles, toPbCard),
		Total:    res.Total,
		Facets:   slicex.Map(res.Facets, toPbFacet),
	}, nil
}

func (s *SearchServer) IndexArticle(ctx context.Context, req *searchv1.IndexArticleRequest) (*searchv1.IndexArticleResponse, error) {
	if err := s.articleSvc.Index(ctx, toDomainDoc(req.GetDoc())); err != nil {
		return nil, err
	}
	return &searchv1.IndexArticleResponse{}, nil
}

func (s *SearchServer) RemoveArticle(ctx context.Context, req *searchv1.RemoveArticleRequest) (*searchv1.RemoveArticleResponse, error) {
	if err := s.articleSvc.Remove(ctx, req.GetId()); err != nil {
		return nil, err
	}
	return &searchv1.RemoveArticleResponse{}, nil
}

func (s *SearchServer) RecommendTags(ctx context.Context, req *searchv1.RecommendTagsRequest) (*searchv1.RecommendTagsResponse, error) {
	tags, err := s.articleSvc.RecommendTags(ctx, req.GetTitle(), req.GetContent(), int(req.GetK()))
	if err != nil {
		return nil, err
	}
	return &searchv1.RecommendTagsResponse{Tags: slicex.Map(tags, toPbFacet)}, nil
}

// ── domain ↔ pb 单条转换（批量走 slicex.Map）─────────────
func toPbCard(a domain.Article) *searchv1.ArticleCard {
	return &searchv1.ArticleCard{
		Id:         a.Id,
		Title:      a.Title,
		Abstract:   a.Abstract,
		AuthorId:   a.AuthorId,
		AuthorName: a.AuthorName,
		Category:   a.Category,
		Tags:       a.Tags,
		CreatedAt:  a.CreatedAt,
	}
}

func toPbFacet(t domain.TagCount) *searchv1.TagCount {
	return &searchv1.TagCount{Slug: t.Slug, Count: t.Count}
}

func toDomainDoc(d *searchv1.ArticleDoc) domain.Article {
	if d == nil {
		return domain.Article{}
	}
	return domain.Article{
		Id:         d.GetId(),
		Title:      d.GetTitle(),
		Abstract:   d.GetAbstract(),
		AuthorId:   d.GetAuthorId(),
		AuthorName: d.GetAuthorName(),
		Status:     uint8(d.GetStatus()),
		Category:   d.GetCategory(),
		Tags:       d.GetTags(),
		CreatedAt:  d.GetCreatedAt(),
		Content:    d.GetContent(),
	}
}
