package service

import (
	"context"

	"gitee.com/train-cloud/geektime-basic-go/internal/domain"
	"gitee.com/train-cloud/geektime-basic-go/internal/repository"
)

type InteractionService interface {
	IncrReadCount(ctx context.Context, biz string, bizId int64) error
	Like(ctx context.Context, uid int64, biz string, bizId int64) error
	CancelLike(ctx context.Context, uid int64, biz string, bizId int64) error
	Collect(ctx context.Context, uid int64, biz string, bizId int64) error
	CancelCollect(ctx context.Context, uid int64, biz string, bizId int64) error
	FindInteraction(ctx context.Context, uid int64, biz string, bizId int64) (domain.Interaction, error)
	// FindByBizIds 批量查聚合计数，返回 map[bizId]Interaction
	FindByBizIds(ctx context.Context, biz string, bizIds []int64) (map[int64]domain.Interaction, error)
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
