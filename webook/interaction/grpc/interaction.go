package grpc

import (
	"context"

	interactionv1 "github.com/boyxs/train-go/webook/api/gen/interaction/v1"
	"github.com/boyxs/train-go/webook/interaction/domain"
	"github.com/boyxs/train-go/webook/interaction/service"
	"github.com/boyxs/train-go/webook/pkg/slicex"
)

// InteractionServer 把 InteractionService 适配成 gRPC 接口。
// 入参校验由 ValidateUnaryInterceptor 统一做；错误 return *errs.Error，由 errconv 拦截器转 status。
type InteractionServer struct {
	interactionv1.UnimplementedInteractionServiceServer
	svc service.InteractionService
}

func NewInteractionServer(svc service.InteractionService) *InteractionServer {
	return &InteractionServer{svc: svc}
}

// ── 写 ────────────────────────────────────────────────

func (s *InteractionServer) Like(ctx context.Context, req *interactionv1.LikeRequest) (*interactionv1.LikeResponse, error) {
	if err := s.svc.Like(ctx, req.GetUid(), req.GetBiz(), req.GetBizId()); err != nil {
		return nil, err
	}
	return &interactionv1.LikeResponse{}, nil
}

func (s *InteractionServer) CancelLike(ctx context.Context, req *interactionv1.CancelLikeRequest) (*interactionv1.CancelLikeResponse, error) {
	if err := s.svc.CancelLike(ctx, req.GetUid(), req.GetBiz(), req.GetBizId()); err != nil {
		return nil, err
	}
	return &interactionv1.CancelLikeResponse{}, nil
}

func (s *InteractionServer) Collect(ctx context.Context, req *interactionv1.CollectRequest) (*interactionv1.CollectResponse, error) {
	if err := s.svc.Collect(ctx, req.GetUid(), req.GetBiz(), req.GetBizId()); err != nil {
		return nil, err
	}
	return &interactionv1.CollectResponse{}, nil
}

func (s *InteractionServer) CancelCollect(ctx context.Context, req *interactionv1.CancelCollectRequest) (*interactionv1.CancelCollectResponse, error) {
	if err := s.svc.CancelCollect(ctx, req.GetUid(), req.GetBiz(), req.GetBizId()); err != nil {
		return nil, err
	}
	return &interactionv1.CancelCollectResponse{}, nil
}

// IncrReadCount 阅读/浏览数 +1（无需登录）。
func (s *InteractionServer) IncrReadCount(ctx context.Context, req *interactionv1.IncrReadCountRequest) (*interactionv1.IncrReadCountResponse, error) {
	if err := s.svc.IncrReadCount(ctx, req.GetBiz(), req.GetBizId()); err != nil {
		return nil, err
	}
	return &interactionv1.IncrReadCountResponse{}, nil
}

// BatchIncrReadCount 批量阅读数累加（worker 聚合 read 事件后一次提交）。
func (s *InteractionServer) BatchIncrReadCount(ctx context.Context, req *interactionv1.BatchIncrReadCountRequest) (*interactionv1.BatchIncrReadCountResponse, error) {
	items := slicex.Map(req.GetItems(), func(it *interactionv1.ReadCountItem) domain.ReadCountItem {
		return domain.ReadCountItem{Biz: it.GetBiz(), BizId: it.GetBizId(), Count: it.GetCount()}
	})
	if err := s.svc.BatchIncrReadCount(ctx, items); err != nil {
		return nil, err
	}
	return &interactionv1.BatchIncrReadCountResponse{}, nil
}

// ── 读 ────────────────────────────────────────────────

// GetInteraction 单条聚合计数；uid>0 时回填 liked/collected。
func (s *InteractionServer) GetInteraction(ctx context.Context, req *interactionv1.GetInteractionRequest) (*interactionv1.GetInteractionResponse, error) {
	intr, err := s.svc.FindInteraction(ctx, req.GetUid(), req.GetBiz(), req.GetBizId())
	if err != nil {
		return nil, err
	}
	return &interactionv1.GetInteractionResponse{Interaction: toPb(intr)}, nil
}

func (s *InteractionServer) GetUserState(ctx context.Context, req *interactionv1.GetUserStateRequest) (*interactionv1.GetUserStateResponse, error) {
	liked, collected, err := s.svc.FindUserState(ctx, req.GetUid(), req.GetBiz(), req.GetBizId())
	if err != nil {
		return nil, err
	}
	return &interactionv1.GetUserStateResponse{Liked: liked, Collected: collected}, nil
}

// BatchGetInteractions 按 biz_ids 批量取聚合计数，key=biz_id。
func (s *InteractionServer) BatchGetInteractions(ctx context.Context, req *interactionv1.BatchGetInteractionsRequest) (*interactionv1.BatchGetInteractionsResponse, error) {
	intrMap, err := s.svc.FindByBizIds(ctx, req.GetBiz(), req.GetBizIds())
	if err != nil {
		return nil, err
	}
	pbMap := make(map[int64]*interactionv1.Interaction, len(intrMap))
	for bizId, intr := range intrMap {
		pbMap[bizId] = toPb(intr)
	}
	return &interactionv1.BatchGetInteractionsResponse{Interactions: pbMap}, nil
}

// GetUserLiked 批量查用户对 biz_ids 的点赞状态，只回已赞的 biz_id。
func (s *InteractionServer) GetUserLiked(ctx context.Context, req *interactionv1.GetUserLikedRequest) (*interactionv1.GetUserLikedResponse, error) {
	likedMap, err := s.svc.FindUserLiked(ctx, req.GetUid(), req.GetBiz(), req.GetBizIds())
	if err != nil {
		return nil, err
	}
	likedIds := make([]int64, 0, len(likedMap))
	for id := range likedMap {
		likedIds = append(likedIds, id)
	}
	return &interactionv1.GetUserLikedResponse{LikedBizIds: likedIds}, nil
}

// GetHotBizIds 互动加权分降序前 N 个 bizId（chat 工具 get_hot_articles 用）。
func (s *InteractionServer) GetHotBizIds(ctx context.Context, req *interactionv1.GetHotBizIdsRequest) (*interactionv1.GetHotBizIdsResponse, error) {
	ids, err := s.svc.ListHotBizIds(ctx, req.GetBiz(), normLimit(req.GetLimit()))
	if err != nil {
		return nil, err
	}
	return &interactionv1.GetHotBizIdsResponse{BizIds: ids}, nil
}

// GetCollectedBizIds 取指定用户收藏的前 N 个 bizId（chat 工具 get_my_favorites 用）。
func (s *InteractionServer) GetCollectedBizIds(ctx context.Context, req *interactionv1.GetCollectedBizIdsRequest) (*interactionv1.GetCollectedBizIdsResponse, error) {
	ids, err := s.svc.ListCollectedBizIds(ctx, req.GetUid(), req.GetBiz(), normLimit(req.GetLimit()))
	if err != nil {
		return nil, err
	}
	return &interactionv1.GetCollectedBizIdsResponse{BizIds: ids}, nil
}

func normLimit(limit int32) int {
	n := int(limit)
	if n <= 0 || n > 100 {
		return 10
	}
	return n
}

// toPb 单条 domain → pb（唯一映射点）。
func toPb(i domain.Interaction) *interactionv1.Interaction {
	return &interactionv1.Interaction{
		Biz:          i.Biz,
		BizId:        i.BizId,
		ReadCount:    i.ReadCount,
		LikeCount:    i.LikeCount,
		CollectCount: i.CollectCount,
		Liked:        i.Liked,
		Collected:    i.Collected,
	}
}
