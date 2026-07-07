package repository

import (
	"context"
	"database/sql"

	"github.com/webook/comment/domain"
	"github.com/webook/comment/repository/cache"
	"github.com/webook/comment/repository/dao"
	"github.com/webook/pkg/logger"
	"github.com/webook/pkg/slicex"
)

type CommentRepository interface {
	Create(ctx context.Context, c domain.Comment) (domain.Comment, error)
	FindById(ctx context.Context, id int64) (domain.Comment, error)
	BatchGet(ctx context.Context, ids []int64) ([]domain.Comment, error)
	PageRoots(ctx context.Context, biz string, bizId int64, offset, limit int) ([]domain.Comment, error)
	GetReplies(ctx context.Context, rootId int64, offset, limit int) ([]domain.Comment, error)
	Delete(ctx context.Context, id, uid int64) (bool, error)
	Count(ctx context.Context, biz string, bizId int64) (int64, error)
	BatchCount(ctx context.Context, biz string, bizIds []int64) (map[int64]int64, error)
}

type CacheCommentRepository struct {
	dao   dao.CommentDAO
	cache cache.CommentCache
	l     logger.LoggerX
}

func NewCacheCommentRepository(d dao.CommentDAO, c cache.CommentCache, l logger.LoggerX) CommentRepository {
	return &CacheCommentRepository{dao: d, cache: c, l: l}
}

func (r *CacheCommentRepository) Create(ctx context.Context, c domain.Comment) (domain.Comment, error) {
	entity, err := r.dao.Insert(ctx, r.toEntity(c))
	if err != nil {
		return domain.Comment{}, err
	}
	// 写后清评论总数缓存（Cache-Aside），失败仅记日志：缓存最迟 TTL 后失效，不阻断写
	if delErr := r.cache.DelCount(ctx, c.Biz, c.BizId); delErr != nil {
		r.l.Error("清除评论计数缓存失败",
			logger.String("biz", c.Biz), logger.Int64("bizId", c.BizId), logger.Error(delErr))
	}
	return r.toDomain(entity), nil
}

func (r *CacheCommentRepository) FindById(ctx context.Context, id int64) (domain.Comment, error) {
	c, err := r.dao.FindById(ctx, id)
	if err != nil {
		return domain.Comment{}, err
	}
	return r.toDomain(c), nil
}

func (r *CacheCommentRepository) BatchGet(ctx context.Context, ids []int64) ([]domain.Comment, error) {
	list, err := r.dao.BatchGet(ctx, ids)
	if err != nil {
		return nil, err
	}
	return slicex.Map(list, r.toDomain), nil
}

func (r *CacheCommentRepository) PageRoots(ctx context.Context, biz string, bizId int64, offset, limit int) ([]domain.Comment, error) {
	list, err := r.dao.PageRoots(ctx, biz, bizId, offset, limit)
	if err != nil {
		return nil, err
	}
	return slicex.Map(list, r.toDomain), nil
}

func (r *CacheCommentRepository) GetReplies(ctx context.Context, rootId int64, offset, limit int) ([]domain.Comment, error) {
	list, err := r.dao.ListReplies(ctx, rootId, offset, limit)
	if err != nil {
		return nil, err
	}
	return slicex.Map(list, r.toDomain), nil
}

func (r *CacheCommentRepository) Delete(ctx context.Context, id, uid int64) (bool, error) {
	c, ok, err := r.dao.Delete(ctx, id, uid)
	if err != nil {
		return false, err
	}
	if ok {
		if delErr := r.cache.DelCount(ctx, c.Biz, c.BizId); delErr != nil {
			r.l.Error("清除评论计数缓存失败",
				logger.String("biz", c.Biz), logger.Int64("bizId", c.BizId), logger.Error(delErr))
		}
	}
	return ok, nil
}

func (r *CacheCommentRepository) Count(ctx context.Context, biz string, bizId int64) (int64, error) {
	// Cache-Aside：先查缓存
	n, err := r.cache.GetCount(ctx, biz, bizId)
	if err == nil {
		return n, nil
	}
	if !cache.IsMiss(err) {
		r.l.Error("读评论计数缓存失败",
			logger.String("biz", biz), logger.Int64("bizId", bizId), logger.Error(err))
	}
	// miss 或读缓存出错 → 回源 DB
	n, err = r.dao.Count(ctx, biz, bizId)
	if err != nil {
		return 0, err
	}
	if setErr := r.cache.SetCount(ctx, biz, bizId, n); setErr != nil {
		r.l.Error("回填评论计数缓存失败",
			logger.String("biz", biz), logger.Int64("bizId", bizId), logger.Error(setErr))
	}
	return n, nil
}

// BatchCount 批量走一次 GROUP BY DB 查询（不逐 id 读/回填缓存：他人主页非热点，单查已够快）。
func (r *CacheCommentRepository) BatchCount(ctx context.Context, biz string, bizIds []int64) (map[int64]int64, error) {
	return r.dao.BatchCount(ctx, biz, bizIds)
}

// toEntity / toDomain 是字段映射的唯一真相源；批量一律 slicex.Map(list, r.toDomain)。

func (r *CacheCommentRepository) toEntity(c domain.Comment) dao.Comment {
	e := dao.Comment{
		Id:      c.Id,
		Biz:     c.Biz,
		BizId:   c.BizId,
		Uid:     c.UserId,
		RootId:  c.RootId,
		Content: c.Content,
	}
	if c.Pid != 0 {
		e.Pid = sql.NullInt64{Int64: c.Pid, Valid: true}
	}
	return e
}

func (r *CacheCommentRepository) toDomain(c dao.Comment) domain.Comment {
	return domain.Comment{
		Id:        c.Id,
		Biz:       c.Biz,
		BizId:     c.BizId,
		UserId:    c.Uid,
		Content:   c.Content,
		RootId:    c.RootId,
		Pid:       c.Pid.Int64, // NULL → 0
		ReplyCnt:  c.ReplyCnt,
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
	}
}
