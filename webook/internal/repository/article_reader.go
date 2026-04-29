package repository

import (
	"context"

	"golang.org/x/sync/errgroup"

	"github.com/webook/internal/domain"
	"github.com/webook/internal/repository/cache"
	"github.com/webook/internal/repository/dao"
	"github.com/webook/pkg/logger"
)

// ArticleReaderRepository 线上库 Repository
type ArticleReaderRepository interface {
	Upsert(ctx context.Context, article domain.Article) error
	Delete(ctx context.Context, id int64, uid int64) error
	FindById(ctx context.Context, id int64) (domain.Article, error)
	// FindByIds 批量查；缓存命中部分 id 直接返回，剩余走 IN 查询 + 回填。结果按入参 ids 顺序返回，缺失 id 跳过
	FindByIds(ctx context.Context, ids []int64) ([]domain.Article, error)
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
	if cErr := r.cache.DelPub(ctx, article.Id); cErr != nil {
		r.l.Error("Upsert 后清除公开文章缓存失败", logger.Int64("id", article.Id), logger.Error(cErr))
	}
	return nil
}

func (r *CacheArticleReaderRepository) Delete(ctx context.Context, id int64, uid int64) error {
	err := r.dao.Delete(ctx, id, uid)
	if err != nil {
		return err
	}
	r.delFirstPageCache(ctx)
	if cErr := r.cache.DelPub(ctx, id); cErr != nil {
		r.l.Error("删除公开文章缓存失败", logger.Int64("id", id), logger.Error(cErr))
	}
	return nil
}

func (r *CacheArticleReaderRepository) delFirstPageCache(ctx context.Context) {
	if err := r.cache.DelFirstPage(ctx); err != nil {
		r.l.Error("删除首页缓存失败", logger.Error(err))
	}
}

func (r *CacheArticleReaderRepository) FindById(ctx context.Context, id int64) (domain.Article, error) {
	art, err := r.cache.GetPub(ctx, id)
	if err == nil {
		return art, nil
	}
	pub, err := r.dao.FindById(ctx, id)
	if err != nil {
		return domain.Article{}, err
	}
	result := r.toDomain(pub)
	if cErr := r.cache.SetPub(ctx, result); cErr != nil {
		r.l.Error("回填公开文章缓存失败", logger.Int64("id", id), logger.Error(cErr))
	}
	return result, nil
}

func (r *CacheArticleReaderRepository) FindByIds(ctx context.Context, ids []int64) ([]domain.Article, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	cacheHit, err := r.cache.MGetPub(ctx, ids)
	if err != nil {
		// MGet 整体失败不阻断；当作 0 命中走 DB
		r.l.Error("MGetPub 失败，全量回源 DB", logger.Error(err))
		cacheHit = map[int64]domain.Article{}
	}

	missIds := make([]int64, 0, len(ids))
	for _, id := range ids {
		if _, ok := cacheHit[id]; !ok {
			missIds = append(missIds, id)
		}
	}

	if len(missIds) > 0 {
		// dao.FindByIds 为节省带宽 Select 排除了 Content；这里只填到本次结果，
		// 不回写 article:pub:{id} 缓存——避免污染单条 FindById 的全字段缓存
		// 导致 /article/:id 详情页显示空正文。批量场景靠单条 FindById 自然预热。
		entityList, dErr := r.dao.FindByIds(ctx, missIds)
		if dErr != nil {
			return nil, dErr
		}
		for _, e := range entityList {
			a := r.toDomain(e)
			cacheHit[a.Id] = a
		}
	}

	result := make([]domain.Article, 0, len(ids))
	for _, id := range ids {
		if a, ok := cacheHit[id]; ok {
			result = append(result, a)
		}
	}
	return result, nil
}

func (r *CacheArticleReaderRepository) Page(ctx context.Context, offset int, limit int) ([]domain.Article, int64, error) {
	// 首页走缓存
	if offset == 0 {
		arts, total, err := r.cache.GetFirstPage(ctx)
		if err == nil {
			return arts, total, nil
		}
	}

	// 缓存 miss 或非首页，并发查 DB
	var articles []dao.PublishedArticle
	var count int64
	var eg errgroup.Group
	eg.Go(func() error {
		var e error
		articles, e = r.dao.Page(ctx, offset, limit)
		return e
	})
	eg.Go(func() error {
		var e error
		count, e = r.dao.Count(ctx)
		return e
	})
	if err := eg.Wait(); err != nil {
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
		Id:        a.Id,
		Title:     a.Title,
		Content:   a.Content,
		Abstract:  a.Abstract,
		Author:    domain.Author{Id: a.AuthorId},
		Status:    domain.ArticleStatus(a.Status),
		Category:  a.Category,
		CreatedAt: a.CreatedAt,
		UpdatedAt: a.UpdatedAt,
	}
}
