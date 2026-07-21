package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/boyxs/train-go/webook/interaction/domain"
	"github.com/boyxs/train-go/webook/interaction/repository/cache"
	"github.com/boyxs/train-go/webook/interaction/repository/dao"
	"github.com/boyxs/train-go/webook/pkg/logger"
	"github.com/boyxs/train-go/webook/pkg/slicex"
)

type InteractionRepository interface {
	IncrReadCount(ctx context.Context, biz string, bizId int64) error
	BatchIncrReadCount(ctx context.Context, items []domain.ReadCountItem) error
	Like(ctx context.Context, uid int64, biz string, bizId int64) error
	CancelLike(ctx context.Context, uid int64, biz string, bizId int64) error
	Collect(ctx context.Context, uid int64, biz string, bizId int64) error
	CancelCollect(ctx context.Context, uid int64, biz string, bizId int64) error
	FindInteraction(ctx context.Context, uid int64, biz string, bizId int64) (domain.Interaction, error)
	// FindUserState 只查用户对指定业务的 liked/collected，不查聚合计数
	FindUserState(ctx context.Context, uid int64, biz string, bizId int64) (liked, collected bool, err error)
	FindByBizIds(ctx context.Context, biz string, bizIds []int64) ([]domain.Interaction, error)
	// FindLikedBizIds 批量查用户在 bizIds 中已点赞的 bizId（供列表聚合 liked 状态，避免 N+1）
	FindLikedBizIds(ctx context.Context, uid int64, biz string, bizIds []int64) ([]int64, error)
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
	if cErr := r.cache.IncrReadCntIfPresent(ctx, biz, bizId, 1); cErr != nil {
		r.l.WithContext(ctx).Error("缓存增加阅读量失败",
			logger.String("biz", biz), logger.Int64("bizId", bizId), logger.Error(cErr))
	}
	return nil
}

// BatchIncrReadCount 批量累加阅读数（worker 聚合一批 read 事件后一次提交）。
func (r *CacheInteractionRepository) BatchIncrReadCount(ctx context.Context, items []domain.ReadCountItem) error {
	if len(items) == 0 {
		return nil
	}
	daoItems := slicex.Map(items, func(it domain.ReadCountItem) dao.ReadCountItem {
		return dao.ReadCountItem{Biz: it.Biz, BizId: it.BizId, Count: it.Count}
	})
	if err := r.dao.BatchIncrReadCount(ctx, daoItems); err != nil {
		return err
	}
	// best-effort 回写缓存：present 才 incr（与单条 IncrReadCount 一致），失败只记日志不阻断
	for _, it := range items {
		if cErr := r.cache.IncrReadCntIfPresent(ctx, it.Biz, it.BizId, it.Count); cErr != nil {
			r.l.WithContext(ctx).Error("批量增加阅读量缓存失败",
				logger.String("biz", it.Biz), logger.Int64("bizId", it.BizId), logger.Error(cErr))
		}
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
		r.l.WithContext(ctx).Error("删除用户互动状态缓存失败",
			logger.Int64("uid", uid), logger.Error(err))
	}
}

func (r *CacheInteractionRepository) delCache(ctx context.Context, biz string, bizId int64) {
	if err := r.cache.Del(ctx, biz, bizId); err != nil {
		r.l.WithContext(ctx).Error("删除互动缓存失败",
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
		r.l.WithContext(ctx).Error("回填互动缓存失败",
			logger.String("biz", biz), logger.Int64("bizId", bizId), logger.Error(cErr))
	}
	if uid > 0 {
		r.fillUserState(ctx, uid, biz, bizId, &result)
	}
	return result, nil
}

// FindByBizIds 逐个尝试缓存，miss 的批量回源 DB 再同步回填（复用 caller ctx，不脱离请求生命周期）。
func (r *CacheInteractionRepository) FindByBizIds(ctx context.Context, biz string, bizIds []int64) ([]domain.Interaction, error) {
	result := make([]domain.Interaction, 0, len(bizIds))
	missIds := make([]int64, 0, len(bizIds))

	for _, id := range bizIds {
		intr, err := r.cache.Get(ctx, biz, id)
		if err == nil {
			result = append(result, intr)
			continue
		}
		missIds = append(missIds, id)
	}

	if len(missIds) == 0 {
		return result, nil
	}

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
		// 同步回填，复用 caller ctx。原 fire-and-forget go func + context.Background 有两宗罪：
		// ① 大批量/高 QPS 下无界 spawn goroutine；② 脱离请求生命周期，丢 trace/取消信号。
		// miss 通常是少数，同步回填可接受；单条失败只记日志不阻断整体返回。
		if setErr := r.cache.Set(ctx, intr); setErr != nil {
			r.l.WithContext(ctx).Error("批量查询回填互动缓存失败",
				logger.Int64("bizId", intr.BizId), logger.Error(setErr))
		}
	}
	return result, nil
}

func (r *CacheInteractionRepository) FindLikedBizIds(ctx context.Context, uid int64, biz string, bizIds []int64) ([]int64, error) {
	return r.dao.FindLikedBizIds(ctx, uid, biz, bizIds)
}

func (r *CacheInteractionRepository) ListCollectedBizIds(ctx context.Context, uid int64, biz string, limit int) ([]int64, error) {
	return r.dao.ListCollectedBizIds(ctx, uid, biz, limit)
}

func (r *CacheInteractionRepository) ListHotBizIds(ctx context.Context, biz string, limit int) ([]int64, error) {
	return r.dao.ListHotBizIds(ctx, biz, limit)
}

// fillUserState 作为 FindInteraction 边路逻辑，出错吞掉不阻塞聚合计数返回（FindUserState 内已记日志）。
func (r *CacheInteractionRepository) fillUserState(ctx context.Context, uid int64, biz string, bizId int64, intr *domain.Interaction) {
	liked, collected, _ := r.FindUserState(ctx, uid, biz, bizId)
	intr.Liked = liked
	intr.Collected = collected
}

// FindUserState Cache-Aside 查用户状态。
func (r *CacheInteractionRepository) FindUserState(ctx context.Context, uid int64, biz string, bizId int64) (bool, bool, error) {
	if liked, collected, err := r.cache.GetUserState(ctx, uid, biz, bizId); err == nil {
		return liked, collected, nil
	}
	ui, err := r.dao.FindUserInteraction(ctx, uid, biz, bizId)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		r.l.WithContext(ctx).Error("查询用户互动状态失败",
			logger.Int64("uid", uid), logger.String("biz", biz), logger.Int64("bizId", bizId), logger.Error(err))
		return false, false, err
	}
	if setErr := r.cache.SetUserState(ctx, uid, biz, bizId, ui.Liked, ui.Collected); setErr != nil {
		r.l.WithContext(ctx).Error("回填用户互动状态缓存失败",
			logger.Int64("uid", uid), logger.Error(setErr))
	}
	return ui.Liked, ui.Collected, nil
}
