package repository

import (
	"context"

	"gitee.com/train-cloud/geektime-basic-go/internal/domain"
	"gitee.com/train-cloud/geektime-basic-go/internal/repository/cache"
	"gitee.com/train-cloud/geektime-basic-go/internal/repository/dao"
	"go.uber.org/zap"
)

// ArticleAuthorRepository 制作库 Repository
type ArticleAuthorRepository interface {
	Create(ctx context.Context, article domain.Article) (int64, error)
	Update(ctx context.Context, article domain.Article) error
	UpdateStatus(ctx context.Context, id int64, uid int64, fromStatus uint8, toStatus uint8) error
	FindById(ctx context.Context, id int64, uid int64) (domain.Article, error)
	Page(ctx context.Context, uid int64, offset int, limit int) ([]domain.Article, int64, error)
	List(ctx context.Context, uid int64) ([]domain.Article, error)
	Delete(ctx context.Context, id int64, uid int64) error
}

type CacheArticleAuthorRepository struct {
	dao   dao.ArticleAuthorDAO
	cache cache.ArticleCache
}

func NewCacheArticleAuthorRepository(
	dao dao.ArticleAuthorDAO,
	cache cache.ArticleCache,
) ArticleAuthorRepository {
	return &CacheArticleAuthorRepository{
		dao:   dao,
		cache: cache,
	}
}

func (r *CacheArticleAuthorRepository) Create(ctx context.Context, article domain.Article) (int64, error) {
	return r.dao.Insert(ctx, r.toEntity(article))
}

func (r *CacheArticleAuthorRepository) Update(ctx context.Context, article domain.Article) error {
	err := r.dao.Update(ctx, r.toEntity(article))
	if err != nil {
		return err
	}
	r.delCache(ctx, article.Author.Id, article.Id)
	return nil
}

func (r *CacheArticleAuthorRepository) UpdateStatus(ctx context.Context, id int64, uid int64, fromStatus uint8, toStatus uint8) error {
	return r.dao.UpdateStatus(ctx, id, uid, fromStatus, toStatus)
}

func (r *CacheArticleAuthorRepository) FindById(ctx context.Context, id int64, uid int64) (domain.Article, error) {
	art, err := r.cache.Get(ctx, uid, id)
	if err == nil {
		return art, nil
	}
	article, err := r.dao.FindByIdAndAuthor(ctx, id, uid)
	if err != nil {
		return domain.Article{}, err
	}
	result := r.toDomain(article)
	r.setCache(ctx, result)
	return result, nil
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
	err := r.dao.Delete(ctx, id, uid)
	if err != nil {
		return err
	}
	r.delCache(ctx, uid, id)
	return nil
}

func (r *CacheArticleAuthorRepository) delCache(ctx context.Context, uid int64, id int64) {
	if err := r.cache.Del(ctx, uid, id); err != nil {
		zap.L().Error("删除文章缓存失败", zap.Int64("uid", uid), zap.Int64("id", id), zap.Error(err))
	}
}

func (r *CacheArticleAuthorRepository) setCache(ctx context.Context, article domain.Article) {
	if err := r.cache.Set(ctx, article); err != nil {
		zap.L().Error("设置文章缓存失败", zap.Int64("uid", article.Author.Id), zap.Int64("id", article.Id), zap.Error(err))
	}
}

func (r *CacheArticleAuthorRepository) toDomain(a dao.Article) domain.Article {
	return domain.Article{
		Id:        a.Id,
		Title:     a.Title,
		Content:   a.Content,
		Abstract:  a.Abstract,
		Author:    domain.Author{Id: a.AuthorId},
		Status:    domain.ArticleStatus(a.Status),
		CreatedAt: a.CreatedAt,
		UpdatedAt: a.UpdatedAt,
	}
}

func (r *CacheArticleAuthorRepository) toEntity(a domain.Article) dao.Article {
	return dao.Article{
		Id:       a.Id,
		Title:    a.Title,
		Content:  a.Content,
		Abstract: a.Abstract,
		AuthorId: a.Author.Id,
		Status:   a.Status.ToUint8(),
	}
}
