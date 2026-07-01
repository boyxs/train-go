package service

import (
	"context"

	interactionv1 "github.com/webook/api/gen/interaction/v1"
	"github.com/webook/internal/domain"
)

type InteractionService interface {
	IncrReadCount(ctx context.Context, biz string, bizId int64) error
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

// GRPCInteractionService 网关适配器：InteractionService 接口转调远端 interaction 服务。
// ranking / Kafka 横切由外层装饰器追加（见 ioc/interaction.go）。
type GRPCInteractionService struct {
	client interactionv1.InteractionServiceClient
}

func NewGRPCInteractionService(client interactionv1.InteractionServiceClient) InteractionService {
	return &GRPCInteractionService{client: client}
}

func (s *GRPCInteractionService) IncrReadCount(ctx context.Context, biz string, bizId int64) error {
	_, err := s.client.IncrReadCount(ctx, &interactionv1.IncrReadCountRequest{Biz: biz, BizId: bizId})
	return err
}

func (s *GRPCInteractionService) Like(ctx context.Context, uid int64, biz string, bizId int64) error {
	_, err := s.client.Like(ctx, &interactionv1.LikeRequest{Uid: uid, Biz: biz, BizId: bizId})
	return err
}

func (s *GRPCInteractionService) CancelLike(ctx context.Context, uid int64, biz string, bizId int64) error {
	_, err := s.client.CancelLike(ctx, &interactionv1.CancelLikeRequest{Uid: uid, Biz: biz, BizId: bizId})
	return err
}

func (s *GRPCInteractionService) Collect(ctx context.Context, uid int64, biz string, bizId int64) error {
	_, err := s.client.Collect(ctx, &interactionv1.CollectRequest{Uid: uid, Biz: biz, BizId: bizId})
	return err
}

func (s *GRPCInteractionService) CancelCollect(ctx context.Context, uid int64, biz string, bizId int64) error {
	_, err := s.client.CancelCollect(ctx, &interactionv1.CancelCollectRequest{Uid: uid, Biz: biz, BizId: bizId})
	return err
}

func (s *GRPCInteractionService) FindInteraction(ctx context.Context, uid int64, biz string, bizId int64) (domain.Interaction, error) {
	resp, err := s.client.GetInteraction(ctx, &interactionv1.GetInteractionRequest{Uid: uid, Biz: biz, BizId: bizId})
	if err != nil {
		return domain.Interaction{}, err
	}
	return toDomain(resp.GetInteraction()), nil
}

func (s *GRPCInteractionService) FindUserState(ctx context.Context, uid int64, biz string, bizId int64) (bool, bool, error) {
	resp, err := s.client.GetUserState(ctx, &interactionv1.GetUserStateRequest{Uid: uid, Biz: biz, BizId: bizId})
	if err != nil {
		return false, false, err
	}
	return resp.GetLiked(), resp.GetCollected(), nil
}

func (s *GRPCInteractionService) FindByBizIds(ctx context.Context, biz string, bizIds []int64) (map[int64]domain.Interaction, error) {
	resp, err := s.client.BatchGetInteractions(ctx, &interactionv1.BatchGetInteractionsRequest{Biz: biz, BizIds: bizIds})
	if err != nil {
		return nil, err
	}
	result := make(map[int64]domain.Interaction, len(resp.GetInteractions()))
	for bizId, intr := range resp.GetInteractions() {
		result[bizId] = toDomain(intr)
	}
	return result, nil
}

func (s *GRPCInteractionService) FindUserLiked(ctx context.Context, uid int64, biz string, bizIds []int64) (map[int64]bool, error) {
	if len(bizIds) == 0 {
		return map[int64]bool{}, nil
	}
	resp, err := s.client.GetUserLiked(ctx, &interactionv1.GetUserLikedRequest{Uid: uid, Biz: biz, BizIds: bizIds})
	if err != nil {
		return nil, err
	}
	result := make(map[int64]bool, len(resp.GetLikedBizIds()))
	for _, id := range resp.GetLikedBizIds() {
		result[id] = true
	}
	return result, nil
}

func (s *GRPCInteractionService) ListHotBizIds(ctx context.Context, biz string, limit int) ([]int64, error) {
	resp, err := s.client.GetHotBizIds(ctx, &interactionv1.GetHotBizIdsRequest{Biz: biz, Limit: int32(limit)})
	if err != nil {
		return nil, err
	}
	return resp.GetBizIds(), nil
}

func (s *GRPCInteractionService) ListCollectedBizIds(ctx context.Context, uid int64, biz string, limit int) ([]int64, error) {
	resp, err := s.client.GetCollectedBizIds(ctx, &interactionv1.GetCollectedBizIdsRequest{Uid: uid, Biz: biz, Limit: int32(limit)})
	if err != nil {
		return nil, err
	}
	return resp.GetBizIds(), nil
}

// toDomain pb → domain 单条转换（唯一映射点）。
func toDomain(i *interactionv1.Interaction) domain.Interaction {
	if i == nil {
		return domain.Interaction{}
	}
	return domain.Interaction{
		Biz:          i.GetBiz(),
		BizId:        i.GetBizId(),
		ReadCount:    i.GetReadCount(),
		LikeCount:    i.GetLikeCount(),
		CollectCount: i.GetCollectCount(),
		Liked:        i.GetLiked(),
		Collected:    i.GetCollected(),
	}
}
