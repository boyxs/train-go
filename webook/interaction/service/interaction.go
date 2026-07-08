package service

import (
	"context"

	"github.com/boyxs/train-go/webook/interaction/domain"
	"github.com/boyxs/train-go/webook/interaction/repository"
)

type InteractionService interface {
	IncrReadCount(ctx context.Context, biz string, bizId int64) error
	// BatchIncrReadCount 批量阅读数累加（worker 聚合一批 read 事件后一次提交）
	BatchIncrReadCount(ctx context.Context, items []domain.ReadCountItem) error
	Like(ctx context.Context, uid int64, biz string, bizId int64) error
	CancelLike(ctx context.Context, uid int64, biz string, bizId int64) error
	Collect(ctx context.Context, uid int64, biz string, bizId int64) error
	CancelCollect(ctx context.Context, uid int64, biz string, bizId int64) error
	FindInteraction(ctx context.Context, uid int64, biz string, bizId int64) (domain.Interaction, error)
	// FindUserState 只查用户的 liked/collected，不查聚合计数
	FindUserState(ctx context.Context, uid int64, biz string, bizId int64) (liked, collected bool, err error)
	// FindByBizIds 批量查聚合计数，返回 map[bizId]Interaction
	FindByBizIds(ctx context.Context, biz string, bizIds []int64) (map[int64]domain.Interaction, error)
	// FindUserLiked 批量查用户对 bizIds 的点赞状态，返回 map[bizId]bool（只含已赞=true，未赞缺省取 false）
	FindUserLiked(ctx context.Context, uid int64, biz string, bizIds []int64) (map[int64]bool, error)
	// ListHotBizIds 按互动加权分降序，取前 limit 个 bizId（chat 工具 get_hot_articles 用）
	ListHotBizIds(ctx context.Context, biz string, limit int) ([]int64, error)
	// ListCollectedBizIds 按用户收藏时间降序，取前 limit 个 bizId（chat 工具 get_my_favorites 用）
	ListCollectedBizIds(ctx context.Context, uid int64, biz string, limit int) ([]int64, error)
}

type InternalInteractionService struct {
	repo repository.InteractionRepository
}

func NewInternalInteractionService(repo repository.InteractionRepository) InteractionService {
	return &InternalInteractionService{repo: repo}
}

func (s *InternalInteractionService) IncrReadCount(ctx context.Context, biz string, bizId int64) error {
	return s.repo.IncrReadCount(ctx, biz, bizId)
}

func (s *InternalInteractionService) BatchIncrReadCount(ctx context.Context, items []domain.ReadCountItem) error {
	return s.repo.BatchIncrReadCount(ctx, items)
}

func (s *InternalInteractionService) Like(ctx context.Context, uid int64, biz string, bizId int64) error {
	return s.repo.Like(ctx, uid, biz, bizId)
}

func (s *InternalInteractionService) CancelLike(ctx context.Context, uid int64, biz string, bizId int64) error {
	return s.repo.CancelLike(ctx, uid, biz, bizId)
}

func (s *InternalInteractionService) Collect(ctx context.Context, uid int64, biz string, bizId int64) error {
	return s.repo.Collect(ctx, uid, biz, bizId)
}

func (s *InternalInteractionService) CancelCollect(ctx context.Context, uid int64, biz string, bizId int64) error {
	return s.repo.CancelCollect(ctx, uid, biz, bizId)
}

func (s *InternalInteractionService) FindInteraction(ctx context.Context, uid int64, biz string, bizId int64) (domain.Interaction, error) {
	return s.repo.FindInteraction(ctx, uid, biz, bizId)
}

func (s *InternalInteractionService) FindUserState(ctx context.Context, uid int64, biz string, bizId int64) (bool, bool, error) {
	return s.repo.FindUserState(ctx, uid, biz, bizId)
}

func (s *InternalInteractionService) FindByBizIds(ctx context.Context, biz string, bizIds []int64) (map[int64]domain.Interaction, error) {
	intrs, err := s.repo.FindByBizIds(ctx, biz, bizIds)
	if err != nil {
		return nil, err
	}
	result := make(map[int64]domain.Interaction, len(intrs))
	for _, intr := range intrs {
		result[intr.BizId] = intr
	}
	return result, nil
}

func (s *InternalInteractionService) FindUserLiked(ctx context.Context, uid int64, biz string, bizIds []int64) (map[int64]bool, error) {
	if len(bizIds) == 0 {
		return map[int64]bool{}, nil
	}
	likedIds, err := s.repo.FindLikedBizIds(ctx, uid, biz, bizIds)
	if err != nil {
		return nil, err
	}
	result := make(map[int64]bool, len(likedIds))
	for _, id := range likedIds {
		result[id] = true
	}
	return result, nil
}

func (s *InternalInteractionService) ListHotBizIds(ctx context.Context, biz string, limit int) ([]int64, error) {
	return s.repo.ListHotBizIds(ctx, biz, limit)
}

func (s *InternalInteractionService) ListCollectedBizIds(ctx context.Context, uid int64, biz string, limit int) ([]int64, error) {
	return s.repo.ListCollectedBizIds(ctx, uid, biz, limit)
}
