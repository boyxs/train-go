package grpc

import (
	"context"

	interactionv1 "github.com/webook/api/gen/interaction/v1"
	"github.com/webook/internal/service"
	"github.com/webook/pkg/errs"
)

// InteractionServer 把内部 InteractionService 适配成 gRPC 接口。
// 严格走 Handler → Service → Repository 分层，不跨层调 repo。
// 错误处理：return *errs.Error，由 grpcx server interceptor 统一转 status.Status。
type InteractionServer struct {
	interactionv1.UnimplementedInteractionServiceServer
	svc service.InteractionService
}

func NewInteractionServer(svc service.InteractionService) *InteractionServer {
	return &InteractionServer{svc: svc}
}

// GetHotBizIds 取按互动加权分降序的前 N 个 bizId（chat 工具 get_hot_articles 用）。
// 内部转发 repo.ListHotBizIds —— Go DAO 风格用 List 前缀，proto 接口按 AIP 用 Get 前缀。
func (s *InteractionServer) GetHotBizIds(ctx context.Context, req *interactionv1.GetHotBizIdsRequest) (*interactionv1.GetHotBizIdsResponse, error) {
	if req.GetBiz() == "" {
		return nil, errs.New(400, "biz 不能为空")
	}
	limit := int(req.GetLimit())
	if limit <= 0 || limit > 100 {
		limit = 10
	}
	ids, err := s.svc.ListHotBizIds(ctx, req.GetBiz(), limit)
	if err != nil {
		return nil, err
	}
	return &interactionv1.GetHotBizIdsResponse{BizIds: ids}, nil
}

// GetCollectedBizIds 取指定用户收藏的前 N 个 bizId（chat 工具 get_my_favorites 用）。
func (s *InteractionServer) GetCollectedBizIds(ctx context.Context, req *interactionv1.GetCollectedBizIdsRequest) (*interactionv1.GetCollectedBizIdsResponse, error) {
	if req.GetUid() <= 0 {
		return nil, errs.New(400, "uid 必须为正整数")
	}
	if req.GetBiz() == "" {
		return nil, errs.New(400, "biz 不能为空")
	}
	limit := int(req.GetLimit())
	if limit <= 0 || limit > 100 {
		limit = 10
	}
	ids, err := s.svc.ListCollectedBizIds(ctx, req.GetUid(), req.GetBiz(), limit)
	if err != nil {
		return nil, err
	}
	return &interactionv1.GetCollectedBizIdsResponse{BizIds: ids}, nil
}
