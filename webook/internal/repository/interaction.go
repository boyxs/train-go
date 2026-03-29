package repository

import (
	"context"
	"errors"

	"gitee.com/train-cloud/geektime-basic-go/internal/domain"
	"gitee.com/train-cloud/geektime-basic-go/internal/repository/cache"
	"gitee.com/train-cloud/geektime-basic-go/internal/repository/dao"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type InteractionRepository interface {
	IncrReadCount(ctx context.Context, biz string, bizId int64) error
	Like(ctx context.Context, uid int64, biz string, bizId int64) error
	CancelLike(ctx context.Context, uid int64, biz string, bizId int64) error
	Collect(ctx context.Context, uid int64, biz string, bizId int64) error
	CancelCollect(ctx context.Context, uid int64, biz string, bizId int64) error
	FindInteraction(ctx context.Context, uid int64, biz string, bizId int64) (domain.Interaction, error)
	FindByBizIds(ctx context.Context, biz string, bizIds []int64) ([]domain.Interaction, error)
}

type CacheInteractionRepository struct {
	dao   dao.InteractionDAO
	cache cache.InteractionCache
}

func NewCacheInteractionRepository(dao dao.InteractionDAO, cache cache.InteractionCache) InteractionRepository {
	return &CacheInteractionRepository{dao: dao, cache: cache}
}

func (r *CacheInteractionRepository) IncrReadCount(ctx context.Context, biz string, bizId int64) error {
	err := r.dao.IncrReadCount(ctx, biz, bizId)
	if err != nil {
		return err
	}
	if cErr := r.cache.IncrReadCntIfPresent(ctx, biz, bizId); cErr != nil {
		zap.L().Error("缓存增加阅读量失败",
			zap.String("biz", biz), zap.Int64("bizId", bizId), zap.Error(cErr))
	}
	return nil
}

func (r *CacheInteractionRepository) Like(ctx context.Context, uid int64, biz string, bizId int64) error {
	err := r.dao.UpsertLike(ctx, uid, biz, bizId, true)
	if err == nil {
		r.delCache(ctx, biz, bizId)
	}
	return err
}

func (r *CacheInteractionRepository) CancelLike(ctx context.Context, uid int64, biz string, bizId int64) error {
	err := r.dao.UpsertLike(ctx, uid, biz, bizId, false)
	if err == nil {
		r.delCache(ctx, biz, bizId)
	}
	return err
}

func (r *CacheInteractionRepository) Collect(ctx context.Context, uid int64, biz string, bizId int64) error {
	err := r.dao.UpsertCollect(ctx, uid, biz, bizId, true)
	if err == nil {
		r.delCache(ctx, biz, bizId)
	}
	return err
}

func (r *CacheInteractionRepository) CancelCollect(ctx context.Context, uid int64, biz string, bizId int64) error {
	err := r.dao.UpsertCollect(ctx, uid, biz, bizId, false)
	if err == nil {
		r.delCache(ctx, biz, bizId)
	}
	return err
}

func (r *CacheInteractionRepository) delCache(ctx context.Context, biz string, bizId int64) {
	if err := r.cache.Del(ctx, biz, bizId); err != nil {
		zap.L().Error("删除互动缓存失败",
			zap.String("biz", biz), zap.Int64("bizId", bizId), zap.Error(err))
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
		zap.L().Error("回填互动缓存失败",
			zap.String("biz", biz), zap.Int64("bizId", bizId), zap.Error(cErr))
	}
	if uid > 0 {
		r.fillUserState(ctx, uid, biz, bizId, &result)
	}
	return result, nil
}

func (r *CacheInteractionRepository) FindByBizIds(ctx context.Context, biz string, bizIds []int64) ([]domain.Interaction, error) {
	intrs, err := r.dao.FindByBizIds(ctx, biz, bizIds)
	if err != nil {
		return nil, err
	}
	result := make([]domain.Interaction, 0, len(intrs))
	for _, intr := range intrs {
		result = append(result, domain.Interaction{
			BizId:        intr.BizId,
			Biz:          biz,
			ReadCount:    intr.ReadCount,
			LikeCount:    intr.LikeCount,
			CollectCount: intr.CollectCount,
		})
	}
	return result, nil
}

func (r *CacheInteractionRepository) fillUserState(ctx context.Context, uid int64, biz string, bizId int64, intr *domain.Interaction) {
	ui, err := r.dao.FindUserInteraction(ctx, uid, biz, bizId)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		zap.L().Error("查询用户互动状态失败",
			zap.Int64("uid", uid), zap.String("biz", biz), zap.Int64("bizId", bizId), zap.Error(err))
		return
	}
	intr.Liked = ui.Liked
	intr.Collected = ui.Collected
}
