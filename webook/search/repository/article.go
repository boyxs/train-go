package repository

import (
	"context"
	"errors"

	"github.com/boyxs/train-go/webook/pkg/slicex"
	"github.com/boyxs/train-go/webook/search/domain"
	"github.com/boyxs/train-go/webook/search/errs"
	"github.com/boyxs/train-go/webook/search/repository/dao"
)

type ArticleRepository interface {
	Index(ctx context.Context, article domain.Article, vec []float32) error
	Remove(ctx context.Context, id int64) error
	Search(ctx context.Context, text string, vec []float32, filterTags []string, offset, limit int) (domain.SearchResult, error)
	RecommendTags(ctx context.Context, vec []float32, k int) ([]domain.TagCount, error)
}

type ESArticleRepository struct {
	dao dao.ArticleDAO
}

func NewESArticleRepository(d dao.ArticleDAO) ArticleRepository {
	return &ESArticleRepository{dao: d}
}

func (r *ESArticleRepository) Index(ctx context.Context, article domain.Article, vec []float32) error {
	return r.dao.Upsert(ctx, r.toDoc(article, vec))
}

func (r *ESArticleRepository) Remove(ctx context.Context, id int64) error {
	err := r.dao.Delete(ctx, id)
	if errors.Is(err, dao.ErrESDocNotFound) {
		return errs.ErrSearchDocNotFound
	}
	return err
}

func (r *ESArticleRepository) Search(ctx context.Context, text string, vec []float32, filterTags []string, offset, limit int) (domain.SearchResult, error) {
	docs, total, facets, err := r.dao.Search(ctx, text, vec, filterTags, offset, limit)
	if err != nil {
		return domain.SearchResult{}, err
	}
	return domain.SearchResult{
		Articles: slicex.Map(docs, r.toDomain),
		Total:    total,
		Facets:   slicex.Map(facets, r.toDomainFacet),
	}, nil
}

func (r *ESArticleRepository) RecommendTags(ctx context.Context, vec []float32, k int) ([]domain.TagCount, error) {
	facets, err := r.dao.RecommendTags(ctx, vec, k)
	if err != nil {
		return nil, err
	}
	return slicex.Map(facets, r.toDomainFacet), nil
}

func (r *ESArticleRepository) toDoc(a domain.Article, vec []float32) dao.ArticleESDoc {
	return dao.ArticleESDoc{
		Id: a.Id, Title: a.Title, Abstract: a.Abstract,
		AuthorId: a.AuthorId, AuthorName: a.AuthorName, Status: a.Status,
		Category: a.Category, Tags: a.Tags, CreatedAt: a.CreatedAt, ContentVec: vec,
	}
}

func (r *ESArticleRepository) toDomain(d dao.ArticleESDoc) domain.Article {
	return domain.Article{
		Id: d.Id, Title: d.Title, Abstract: d.Abstract,
		AuthorId: d.AuthorId, AuthorName: d.AuthorName, Status: d.Status,
		Category: d.Category, Tags: d.Tags, CreatedAt: d.CreatedAt,
	}
}

func (r *ESArticleRepository) toDomainFacet(t dao.TagCount) domain.TagCount {
	return domain.TagCount{Slug: t.Slug, Count: t.Count}
}
