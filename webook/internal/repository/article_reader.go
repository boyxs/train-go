package repository

import (
	"context"

	"gitee.com/train-cloud/geektime-basic-go/internal/domain"
	"gitee.com/train-cloud/geektime-basic-go/internal/repository/dao"
)

// ArticleReaderRepository 线上库 Repository
type ArticleReaderRepository interface {
	Upsert(ctx context.Context, article domain.Article) error
	Delete(ctx context.Context, id int64, uid int64) error
	FindById(ctx context.Context, id int64) (domain.Article, error)
	Page(ctx context.Context, offset int, limit int) ([]domain.Article, int64, error)
}

type CacheArticleReaderRepository struct {
	dao dao.ArticleReaderDAO
}

func NewCacheArticleReaderRepository(dao dao.ArticleReaderDAO) ArticleReaderRepository {
	return &CacheArticleReaderRepository{dao: dao}
}

func (r *CacheArticleReaderRepository) Upsert(ctx context.Context, article domain.Article) error {
	return r.dao.Upsert(ctx, dao.PublishedArticle{
		Id:       article.Id,
		Title:    article.Title,
		Content:  article.Content,
		AuthorId: article.Author.Id,
		Status:   article.Status.ToUint8(),
	})
}

func (r *CacheArticleReaderRepository) Delete(ctx context.Context, id int64, uid int64) error {
	return r.dao.Delete(ctx, id, uid)
}

func (r *CacheArticleReaderRepository) FindById(ctx context.Context, id int64) (domain.Article, error) {
	pub, err := r.dao.FindById(ctx, id)
	if err != nil {
		return domain.Article{}, err
	}
	return r.toDomain(pub), nil
}

func (r *CacheArticleReaderRepository) Page(ctx context.Context, offset int, limit int) ([]domain.Article, int64, error) {
	articles, err := r.dao.Page(ctx, offset, limit)
	if err != nil {
		return nil, 0, err
	}
	count, err := r.dao.Count(ctx)
	if err != nil {
		return nil, 0, err
	}
	result := make([]domain.Article, 0, len(articles))
	for _, a := range articles {
		result = append(result, r.toDomain(a))
	}
	return result, count, nil
}

func (r *CacheArticleReaderRepository) toDomain(a dao.PublishedArticle) domain.Article {
	return domain.Article{
		Id:      a.Id,
		Title:   a.Title,
		Content: a.Content,
		Author:  domain.Author{Id: a.AuthorId},
		Status:  domain.ArticleStatus(a.Status),
		CreatedAt: a.CreatedAt,
		UpdatedAt: a.UpdatedAt,
	}
}
