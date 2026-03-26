package repository

import (
	"context"

	"gitee.com/train-cloud/geektime-basic-go/internal/domain"
	"gitee.com/train-cloud/geektime-basic-go/internal/repository/dao"
)

type ArticleAuthorRepository interface {
	Create(ctx context.Context, article domain.Article) (int64, error)
	Update(ctx context.Context, article domain.Article) error
	Publish(ctx context.Context, article domain.Article) (int64, error)
	Withdraw(ctx context.Context, id int64, uid int64) error
}

type CacheArticleAuthorRepository struct {
	dao dao.ArticleAuthorDAO
}

func NewCacheArticleAuthorRepository(dao dao.ArticleAuthorDAO) ArticleAuthorRepository {
	return &CacheArticleAuthorRepository{
		dao: dao,
	}
}

func (r *CacheArticleAuthorRepository) Create(ctx context.Context, article domain.Article) (int64, error) {
	id, err := r.dao.Insert(ctx, r.toEntity(article))
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (r *CacheArticleAuthorRepository) Update(ctx context.Context, article domain.Article) error {
	return r.dao.Update(ctx, r.toEntity(article))
}

func (r *CacheArticleAuthorRepository) Publish(ctx context.Context, article domain.Article) (int64, error) {
	return r.dao.Publish(ctx, r.toEntity(article), r.toReaderEntity(article))
}

func (r *CacheArticleAuthorRepository) Withdraw(ctx context.Context, id int64, uid int64) error {
	return r.dao.Withdraw(ctx, id, uid,
		domain.ArticleStatusPublished.ToUint8(),
		domain.ArticleStatusPrivate.ToUint8())
}

func (r *CacheArticleAuthorRepository) toEntity(a domain.Article) dao.Article {
	return dao.Article{
		Id:       a.Id,
		Title:    a.Title,
		Content:  a.Content,
		AuthorId: a.Author.Id,
		Status:   a.Status.ToUint8(),
	}
}

func (r *CacheArticleAuthorRepository) toReaderEntity(a domain.Article) dao.PublishedArticle {
	return dao.PublishedArticle{
		Id:       a.Id,
		Title:    a.Title,
		Content:  a.Content,
		AuthorId: a.Author.Id,
		Status:   a.Status.ToUint8(),
	}
}
