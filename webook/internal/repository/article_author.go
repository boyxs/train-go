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
	FindById(ctx context.Context, id int64, uid int64) (domain.Article, error)
	Page(ctx context.Context, uid int64, offset int, limit int) ([]domain.Article, int64, error)
	List(ctx context.Context, uid int64) ([]domain.Article, error)
	Delete(ctx context.Context, id int64, uid int64) error
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

func (r *CacheArticleAuthorRepository) FindById(ctx context.Context, id int64, uid int64) (domain.Article, error) {
	article, err := r.dao.FindByIdAndAuthor(ctx, id, uid)
	if err != nil {
		return domain.Article{}, err
	}
	return r.toDomain(article), nil
}

func (r *CacheArticleAuthorRepository) Page(ctx context.Context, uid int64, offset int, limit int) ([]domain.Article, int64, error) {
	articles, err := r.dao.PageByAuthor(ctx, uid, offset, limit)
	if err != nil {
		return nil, 0, err
	}
	count, err := r.dao.CountByAuthor(ctx, uid)
	if err != nil {
		return nil, 0, err
	}
	result := make([]domain.Article, 0, len(articles))
	for _, a := range articles {
		result = append(result, r.toDomain(a))
	}
	return result, count, nil
}

func (r *CacheArticleAuthorRepository) List(ctx context.Context, uid int64) ([]domain.Article, error) {
	articles, err := r.dao.ListByAuthor(ctx, uid)
	if err != nil {
		return nil, err
	}
	result := make([]domain.Article, 0, len(articles))
	for _, a := range articles {
		result = append(result, r.toDomain(a))
	}
	return result, nil
}

func (r *CacheArticleAuthorRepository) Delete(ctx context.Context, id int64, uid int64) error {
	return r.dao.Delete(ctx, id, uid)
}

func (r *CacheArticleAuthorRepository) toDomain(a dao.Article) domain.Article {
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
