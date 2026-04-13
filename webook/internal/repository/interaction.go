package repository

import (
	"context"
	"errors"

	"gitee.com/train-cloud/geektime-basic-go/internal/domain"
	"gitee.com/train-cloud/geektime-basic-go/internal/repository/cache"
	"gitee.com/train-cloud/geektime-basic-go/internal/repository/dao"
	"gitee.com/train-cloud/geektime-basic-go/pkg/logger"
	"gorm.io/gorm"
)

type InteractionRepository interface {
	IncrReadCount(ctx context.Context, biz string, bizId int64) error
	Like(ctx context.Context, uid int64, biz string, bizId int64) error
	CancelLike(ctx context.Context, uid int64, biz string, bizId int64) error
	Collect(ctx context.Context, uid int64, biz string, bizId int64) error
	CancelCollect(ctx context.Context, uid int64, biz string, bizId int64) error
	FindInteraction(ctx context.Context, uid int64, biz string, bizId int64) (domain.Interaction, error)
	// FindUserState 只查用户对指定业务的 liked/collected，不查聚合计数
	FindUserState(ctx context.Context, uid int64, biz string, bizId int64) (liked, collected bool, err error)
	FindByBizIds(ctx context.Context, biz string, bizIds []int64) ([]domain.Interaction, error)
	// ListCollectedBizIds 查询用户收藏的 bizId 列表，按收藏时间降序
	ListCollectedBizIds(ctx context.Context, uid int64, biz string, limit int) ([]int64, error)
	// ListHotBizIds 查询热门 bizId 列表，按互动加权分降序
	ListHotBizIds(ctx context.Context, biz string, limit int) ([]int64, error)
}

type CacheInteractionRepository struct {
	dao   dao.InteractionDAO
	cache cache.InteractionCache
	l     logger.LoggerX
}

func NewCacheInteractionRepository(dao dao.InteractionDAO, cache cache.InteractionCache, l logger.LoggerX) InteractionRepository {
	return &CacheInteractionRepository{dao: dao, cache: cache, l: l}
}

func (r *CacheInteractionRepository) IncrReadCount(ctx context.Context, biz string, bizId int64) error {
	err := r.dao.IncrReadCount(ctx, biz, bizId)
	if err != nil {
		return err
	}
	if cErr := r.cache.IncrReadCntIfPresent(ctx, biz, bizId); cErr != nil {
		r.l.Error("缓存增加阅读量失败",
			logger.String("biz", biz), logger.Int64("bizId", bizId), logger.Error(cErr))
	}
	return nil
}

func (r *CacheInteractionRepository) Like(ctx context.Context, uid int64, biz string, bizId int64) error {
	err := r.dao.UpsertLike(ctx, uid, biz, bizId, true)
	if err == nil {
		r.delCache(ctx, biz, bizId)
		r.delUserStateCache(ctx, uid, biz, bizId)
	}
	return err
}

func (r *CacheInteractionRepository) CancelLike(ctx context.Context, uid int64, biz string, bizId int64) error {
	err := r.dao.UpsertLike(ctx, uid, biz, bizId, false)
	if err == nil {
		r.delCache(ctx, biz, bizId)
		r.delUserStateCache(ctx, uid, biz, bizId)
	}
	return err
}

func (r *CacheInteractionRepository) Collect(ctx context.Context, uid int64, biz string, bizId int64) error {
	err := r.dao.UpsertCollect(ctx, uid, biz, bizId, true)
	if err == nil {
		r.delCache(ctx, biz, bizId)
		r.delUserStateCache(ctx, uid, biz, bizId)
	}
	return err
}

func (r *CacheInteractionRepository) CancelCollect(ctx context.Context, uid int64, biz string, bizId int64) error {
	err := r.dao.UpsertCollect(ctx, uid, biz, bizId, false)
	if err == nil {
		r.delCache(ctx, biz, bizId)
		r.delUserStateCache(ctx, uid, biz, bizId)
	}
	return err
}

func (r *CacheInteractionRepository) delUserStateCache(ctx context.Context, uid int64, biz string, bizId int64) {
	if err := r.cache.DelUserState(ctx, uid, biz, bizId); err != nil {
		r.l.Error("删除用户互动状态缓存失败",
			logger.Int64("uid", uid), logger.Error(err))
	}
}

func (r *CacheInteractionRepository) delCache(ctx context.Context, biz string, bizId int64) {
	if err := r.cache.Del(ctx, biz, bizId); err != nil {
		r.l.Error("删除互动缓存失败",
			logger.String("biz", biz), logger.Int64("bizId", bizId), logger.Error(err))
	}
}

func (r *CacheInteractionRepository) FindInteraction(ctx context.Context, uid int64, biz string, bizId int64) (domain.Interaction, error) {
	intr, err := r.cache.Get(ctx, biz, bizId)
	if err == nil {
		if uid > 0 {
			r.fillUserState(ctx, uid, biz, bizId, &intr)
		}
		return intr, nil
	}

	daoIntr, err := r.dao.FindByBizId(ctx, biz, bizId)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.Interaction{}, err
	}
	result := domain.Interaction{
		BizId:        bizId,
		Biz:          biz,
		ReadCount:    daoIntr.ReadCount,
		LikeCount:    daoIntr.LikeCount,
		CollectCount: daoIntr.CollectCount,
	}
	if cErr := r.cache.Set(ctx, result); cErr != nil {
		r.l.Error("回填互动缓存失败",
			logger.String("biz", biz), logger.Int64("bizId", bizId), logger.Error(cErr))
	}
	if uid > 0 {
		r.fillUserState(ctx, uid, biz, bizId, &result)
	}
	return result, nil
}

// FindByBizIds 批量查聚合计数
// 策略：先逐个尝试缓存，miss 的 bizId 批量回源 DB，再异步回填缓存
func (r *CacheInteractionRepository) FindByBizIds(ctx context.Context, biz string, bizIds []int64) ([]domain.Interaction, error) {
	result := make([]domain.Interaction, 0, len(bizIds))
	missIds := make([]int64, 0, len(bizIds))

	// 逐个尝试缓存
	for _, id := range bizIds {
		intr, err := r.cache.Get(ctx, biz, id)
		if err == nil {
			result = append(result, intr)
			continue
		}
		missIds = append(missIds, id)
	}

	// 全部命中直接返回
	if len(missIds) == 0 {
		return result, nil
	}

	// miss 的批量查 DB
	intrs, err := r.dao.FindByBizIds(ctx, biz, missIds)
	if err != nil {
		return nil, err
	}
	for _, e := range intrs {
		intr := domain.Interaction{
			BizId:        e.BizId,
			Biz:          biz,
			ReadCount:    e.ReadCount,
			LikeCount:    e.LikeCount,
			CollectCount: e.CollectCount,
		}
		result = append(result, intr)
		// 异步回填缓存，不阻塞主流程
		go func(i domain.Interaction) {
			if setErr := r.cache.Set(context.Background(), i); setErr != nil {
				r.l.Error("批量查询回填互动缓存失败",
					logger.Int64("bizId", i.BizId), logger.Error(setErr))
			}
		}(intr)
	}
	return result, nil
}

func (r *CacheInteractionRepository) ListCollectedBizIds(ctx context.Context, uid int64, biz string, limit int) ([]int64, error) {
	return r.dao.ListCollectedBizIds(ctx, uid, biz, limit)
}

func (r *CacheInteractionRepository) ListHotBizIds(ctx context.Context, biz string, limit int) ([]int64, error) {
	return r.dao.ListHotBizIds(ctx, biz, limit)
}

// fillUserState 作为 FindInteraction 的边路逻辑，出错不影响聚合计数返回
// FindUserState 内部已记日志，这里刻意吞掉错误保证主流程不被阻塞
func (r *CacheInteractionRepository) fillUserState(ctx context.Context, uid int64, biz string, bizId int64, intr *domain.Interaction) {
	liked, collected, _ := r.FindUserState(ctx, uid, biz, bizId)
	intr.Liked = liked
	intr.Collected = collected
}

// FindUserState Cache-Aside 查用户状态
func (r *CacheInteractionRepository) FindUserState(ctx context.Context, uid int64, biz string, bizId int64) (bool, bool, error) {
	// 缓存优先
	if liked, collected, err := r.cache.GetUserState(ctx, uid, biz, bizId); err == nil {
		return liked, collected, nil
	}
	// 回源 DB
	ui, err := r.dao.FindUserInteraction(ctx, uid, biz, bizId)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		r.l.Error("查询用户互动状态失败",
			logger.Int64("uid", uid), logger.String("biz", biz), logger.Int64("bizId", bizId), logger.Error(err))
		return false, false, err
	}
	// 回填缓存（错误不阻塞）
	if setErr := r.cache.SetUserState(ctx, uid, biz, bizId, ui.Liked, ui.Collected); setErr != nil {
		r.l.Error("回填用户互动状态缓存失败",
			logger.Int64("uid", uid), logger.Error(setErr))
	}
	return ui.Liked, ui.Collected, nil
}
