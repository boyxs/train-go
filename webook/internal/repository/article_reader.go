package repository

import (
	"context"

	"golang.org/x/sync/errgroup"

	"github.com/boyxs/train-go/webook/internal/domain"
	"github.com/boyxs/train-go/webook/internal/migratorsdk"
	"github.com/boyxs/train-go/webook/internal/repository/cache"
	"github.com/boyxs/train-go/webook/internal/repository/dao"
	"github.com/boyxs/train-go/webook/pkg/logger"
)

// ArticleReaderRepository 线上库 Repository
type ArticleReaderRepository interface {
	Upsert(ctx context.Context, article domain.Article) error
	Delete(ctx context.Context, id int64, uid int64) error
	FindById(ctx context.Context, id int64) (domain.Article, error)
	// FindByIds 批量查；缓存命中部分 id 直接返回，剩余走 IN 查询 + 回填。结果按入参 ids 顺序返回，缺失 id 跳过
	FindByIds(ctx context.Context, ids []int64) ([]domain.Article, error)
	Page(ctx context.Context, offset int, limit int) ([]domain.Article, int64, error)
	// PageByAuthor 某作者已发布文章分页 + 总数（他人主页「TA 的文章」）
	PageByAuthor(ctx context.Context, uid int64, offset int, limit int) ([]domain.Article, int64, error)
	// ListIdsByAuthor 某作者全部已发布文章 id（聚合「获赞」总数）
	ListIdsByAuthor(ctx context.Context, uid int64) ([]int64, error)
}

// CacheArticleReaderRepository 线上库 Repository（含 migratorsdk 双写 / 切读）。
//
// SDK 行为：
//   - 默认 NoOp（migrator.sdk.enabled: false）→ 业务等价旧行为，零 Redis 调用
//   - Redis 实现（启用迁移时）→ Upsert/Delete 按 stage 双写 OLD/NEW；FindById/FindByIds 按 stage+gray 切读
//   - Page 不切（跨侧分页语义不一致），切流期始终走 oldDAO
type CacheArticleReaderRepository struct {
	oldDAO       dao.ArticleReaderDAO
	newDAO       dao.ArticleReaderDAO
	cache        cache.ArticleCache
	switchReader migratorsdk.SwitchReader
	dualWriter   migratorsdk.DualWriter
	taskName     string
	l            logger.LoggerX
}

func NewCacheArticleReaderRepository(
	oldDAO dao.ArticleReaderDAO,
	newDAO dao.ArticleReaderNewDAO,
	c cache.ArticleCache,
	sw migratorsdk.SwitchReader,
	dw migratorsdk.DualWriter,
	taskName migratorsdk.TaskName,
	l logger.LoggerX,
) ArticleReaderRepository {
	return &CacheArticleReaderRepository{
		oldDAO: oldDAO, newDAO: dao.ArticleReaderDAO(newDAO), cache: c,
		switchReader: sw, dualWriter: dw, taskName: string(taskName), l: l,
	}
}

// daoBySide 按 SDK 决策返对应 DAO。
func (r *CacheArticleReaderRepository) daoBySide(side migratorsdk.Side) dao.ArticleReaderDAO {
	if side == migratorsdk.SideNew {
		return r.newDAO
	}
	return r.oldDAO
}

// chooseSide 包装 SwitchReader.ChooseSide，显式处理错误（不直接 `_, _:=` 吞错）。
// 失败时降级 SideOld：业务可用优先；SDK 实现层已对 Redis 故障内部降级，这层错误处理是为接口契约预留的扩展兜底。
func (r *CacheArticleReaderRepository) chooseSide(ctx context.Context, hashKey int64) migratorsdk.Side {
	side, err := r.switchReader.ChooseSide(ctx, r.taskName, hashKey)
	if err != nil {
		r.l.Warn("ChooseSide 失败，降级 SideOld",
			logger.String("task", r.taskName),
			logger.Int64("hash_key", hashKey),
			logger.Error(err))
		return migratorsdk.SideOld
	}
	return side
}

func (r *CacheArticleReaderRepository) Upsert(ctx context.Context, article domain.Article) error {
	entity := dao.PublishedArticle{
		Id:       article.Id,
		Title:    article.Title,
		Content:  article.Content,
		Abstract: article.Abstract,
		AuthorId: article.Author.Id,
		Status:   article.Status.ToUint8(),
	}
	err := r.dualWriter.Write(ctx, r.taskName, func(side migratorsdk.Side) error {
		return r.daoBySide(side).Upsert(ctx, entity)
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
	err := r.dualWriter.Write(ctx, r.taskName, func(side migratorsdk.Side) error {
		return r.daoBySide(side).Delete(ctx, id, uid)
	})
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
	side := r.chooseSide(ctx, id)
	pub, err := r.daoBySide(side).FindById(ctx, id)
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
		// 路由按 missIds[0]：dao.FindByIds 不返 Content，结果不回写单条 :pub:{id} 全字段缓存。
		side := r.chooseSide(ctx, missIds[0])
		entityList, dErr := r.daoBySide(side).FindByIds(ctx, missIds)
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

	// Page 不走 SDK：跨侧分页语义不一致（同 offset/limit 看到不同列表），cutover 期保留 OLD。
	var articles []dao.PublishedArticle
	var count int64
	var eg errgroup.Group
	eg.Go(func() error {
		var e error
		articles, e = r.oldDAO.Page(ctx, offset, limit)
		return e
	})
	eg.Go(func() error {
		var e error
		count, e = r.oldDAO.Count(ctx)
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

// PageByAuthor 他人主页「TA 的文章」：始终走 oldDAO（同 Page 不切 SDK），不缓存（非首页热点）。
func (r *CacheArticleReaderRepository) PageByAuthor(ctx context.Context, uid int64, offset int, limit int) ([]domain.Article, int64, error) {
	var articles []dao.PublishedArticle
	var count int64
	var eg errgroup.Group
	eg.Go(func() error {
		var e error
		articles, e = r.oldDAO.PageByAuthor(ctx, uid, offset, limit)
		return e
	})
	eg.Go(func() error {
		var e error
		count, e = r.oldDAO.CountByAuthor(ctx, uid)
		return e
	})
	if err := eg.Wait(); err != nil {
		return nil, 0, err
	}
	result := make([]domain.Article, 0, len(articles))
	for _, a := range articles {
		result = append(result, r.toDomain(a))
	}
	return result, count, nil
}

func (r *CacheArticleReaderRepository) ListIdsByAuthor(ctx context.Context, uid int64) ([]int64, error) {
	return r.oldDAO.ListIdsByAuthor(ctx, uid)
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
