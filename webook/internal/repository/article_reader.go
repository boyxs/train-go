package repository

import (
	"context"

	"gitee.com/train-cloud/geektime-basic-go/internal/domain"
	"gitee.com/train-cloud/geektime-basic-go/internal/repository/cache"
	"gitee.com/train-cloud/geektime-basic-go/internal/repository/dao"
	"gitee.com/train-cloud/geektime-basic-go/pkg/logger"
)

// ArticleReaderRepository 线上库 Repository
type ArticleReaderRepository interface {
	Upsert(ctx context.Context, article domain.Article) error
	Delete(ctx context.Context, id int64, uid int64) error
	FindById(ctx context.Context, id int64) (domain.Article, error)
	Page(ctx context.Context, offset int, limit int) ([]domain.Article, int64, error)
}

type CacheArticleReaderRepository struct {
	dao   dao.ArticleReaderDAO
	cache cache.ArticleCache
	l     logger.LoggerX
}

func NewCacheArticleReaderRepository(dao dao.ArticleReaderDAO, c cache.ArticleCache, l logger.LoggerX) ArticleReaderRepository {
	return &CacheArticleReaderRepository{dao: dao, cache: c, l: l}
}

func (r *CacheArticleReaderRepository) Upsert(ctx context.Context, article domain.Article) error {
	err := r.dao.Upsert(ctx, dao.PublishedArticle{
		Id:       article.Id,
		Title:    article.Title,
		Content:  article.Content,
		Abstract: article.Abstract,
		AuthorId: article.Author.Id,
		Status:   article.Status.ToUint8(),
	})
	if err != nil {
		return err
	}
	r.delFirstPageCache(ctx)
	return nil
}

func (r *CacheArticleReaderRepository) Delete(ctx context.Context, id int64, uid int64) error {
	err := r.dao.Delete(ctx, id, uid)
	if err != nil {
		return err
	}
	r.delFirstPageCache(ctx)
	return nil
}

func (r *CacheArticleReaderRepository) delFirstPageCache(ctx context.Context) {
	if err := r.cache.DelFirstPage(ctx); err != nil {
		r.l.Error("删除首页缓存失败", logger.Error(err))
	}
}

func (r *CacheArticleReaderRepository) FindById(ctx context.Context, id int64) (domain.Article, error) {
	pub, err := r.dao.FindById(ctx, id)
	if err != nil {
		return domain.Article{}, err
	}
	return r.toDomain(pub), nil
}

func (r *CacheArticleReaderRepository) Page(ctx context.Context, offset int, limit int) ([]domain.Article, int64, error) {
	// 首页走缓存
	if offset == 0 {
		arts, total, err := r.cache.GetFirstPage(ctx)
		if err == nil {
			return arts, total, nil
		}
	}

	// 缓存 miss 或非首页，查 DB
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

	// 首页回填缓存
	if offset == 0 {
		if cErr := r.cache.SetFirstPage(ctx, result, count); cErr != nil {
			r.l.Error("回填首页缓存失败", logger.Error(cErr))
		}
	}

	return result, count, nil
}

func (r *CacheArticleReaderRepository) toDomain(a dao.PublishedArticle) domain.Article {
	return domain.Article{
		Id:       a.Id,
		Title:    a.Title,
		Content:  a.Content,
		Abstract: a.Abstract,
		Author:   domain.Author{Id: a.AuthorId},
		Status:  domain.ArticleStatus(a.Status),
		CreatedAt: a.CreatedAt,
		UpdatedAt: a.UpdatedAt,
	}
}
