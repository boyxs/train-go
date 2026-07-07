package grpc

import (
	"context"

	relationv1 "github.com/webook/api/gen/relation/v1"
	"github.com/webook/pkg/slicex"
	"github.com/webook/relation/domain"
	"github.com/webook/relation/service"
)

// RelationServer 把 RelationService 适配成 gRPC 接口。
// 入参非空校验由 ValidateUnaryInterceptor 统一做；错误 return *errs.Error，由 errconv 拦截器转 status。
type RelationServer struct {
	relationv1.UnimplementedRelationServiceServer
	svc service.RelationService
}

func NewRelationServer(svc service.RelationService) *RelationServer {
	return &RelationServer{svc: svc}
}

// ── 写 ────────────────────────────────────────────────

func (s *RelationServer) Follow(ctx context.Context, req *relationv1.FollowRequest) (*relationv1.FollowResponse, error) {
	changed, err := s.svc.Follow(ctx, req.GetFollowerId(), req.GetFolloweeId())
	if err != nil {
		return nil, err
	}
	return &relationv1.FollowResponse{Changed: changed}, nil
}

func (s *RelationServer) Unfollow(ctx context.Context, req *relationv1.FollowRequest) (*relationv1.FollowResponse, error) {
	changed, err := s.svc.Unfollow(ctx, req.GetFollowerId(), req.GetFolloweeId())
	if err != nil {
		return nil, err
	}
	return &relationv1.FollowResponse{Changed: changed}, nil
}

func (s *RelationServer) Block(ctx context.Context, req *relationv1.BlockRequest) (*relationv1.BlockResponse, error) {
	changed, err := s.svc.Block(ctx, req.GetUid(), req.GetBlockedId())
	if err != nil {
		return nil, err
	}
	return &relationv1.BlockResponse{Changed: changed}, nil
}

func (s *RelationServer) Unblock(ctx context.Context, req *relationv1.BlockRequest) (*relationv1.BlockResponse, error) {
	changed, err := s.svc.Unblock(ctx, req.GetUid(), req.GetBlockedId())
	if err != nil {
		return nil, err
	}
	return &relationv1.BlockResponse{Changed: changed}, nil
}

// ── 读 ────────────────────────────────────────────────

func (s *RelationServer) GetStats(ctx context.Context, req *relationv1.GetStatsRequest) (*relationv1.GetStatsResponse, error) {
	st, err := s.svc.GetStats(ctx, req.GetUid())
	if err != nil {
		return nil, err
	}
	return &relationv1.GetStatsResponse{Stats: toPbStats(st)}, nil
}

func (s *RelationServer) BatchGetStats(ctx context.Context, req *relationv1.BatchGetStatsRequest) (*relationv1.BatchGetStatsResponse, error) {
	m, err := s.svc.BatchGetStats(ctx, req.GetUids())
	if err != nil {
		return nil, err
	}
	pbMap := make(map[int64]*relationv1.RelationStats, len(m))
	for uid, st := range m {
		pbMap[uid] = toPbStats(st)
	}
	return &relationv1.BatchGetStatsResponse{Stats: pbMap}, nil
}

func (s *RelationServer) GetRelation(ctx context.Context, req *relationv1.GetRelationRequest) (*relationv1.GetRelationResponse, error) {
	m, err := s.svc.GetRelation(ctx, req.GetViewerId(), req.GetTargetIds())
	if err != nil {
		return nil, err
	}
	pbMap := make(map[int64]*relationv1.RelationState, len(m))
	for tid, st := range m {
		pbMap[tid] = toPbState(st)
	}
	return &relationv1.GetRelationResponse{States: pbMap}, nil
}

func (s *RelationServer) ListFollowees(ctx context.Context, req *relationv1.ListRequest) (*relationv1.ListFollowResponse, error) {
	edges, next, err := s.svc.ListFollowees(ctx, req.GetUid(), req.GetCursor(), int(req.GetLimit()))
	if err != nil {
		return nil, err
	}
	return &relationv1.ListFollowResponse{Edges: slicex.Map(edges, toPbEdge), NextCursor: next}, nil
}

func (s *RelationServer) ListFollowers(ctx context.Context, req *relationv1.ListRequest) (*relationv1.ListFollowResponse, error) {
	edges, next, err := s.svc.ListFollowers(ctx, req.GetUid(), req.GetCursor(), int(req.GetLimit()))
	if err != nil {
		return nil, err
	}
	return &relationv1.ListFollowResponse{Edges: slicex.Map(edges, toPbEdge), NextCursor: next}, nil
}

func (s *RelationServer) ListBlocks(ctx context.Context, req *relationv1.ListRequest) (*relationv1.ListBlockResponse, error) {
	edges, next, err := s.svc.ListBlocks(ctx, req.GetUid(), req.GetCursor(), int(req.GetLimit()))
	if err != nil {
		return nil, err
	}
	return &relationv1.ListBlockResponse{Edges: slicex.Map(edges, toPbBlock), NextCursor: next}, nil
}

// ── domain → pb 单条转换（方向前缀 toPb + 类型后缀；批量走 slicex.Map）──────────────
func toPbStats(s domain.RelationStats) *relationv1.RelationStats {
	return &relationv1.RelationStats{Uid: s.Uid, FolloweeCnt: s.FolloweeCnt, FollowerCnt: s.FollowerCnt}
}

func toPbState(s domain.RelationState) *relationv1.RelationState {
	return &relationv1.RelationState{
		IsFollowing:  s.IsFollowing,
		IsFollowedBy: s.IsFollowedBy,
		IsBlocked:    s.IsBlocked,
		IsBlockedBy:  s.IsBlockedBy,
	}
}

func toPbEdge(e domain.FollowEdge) *relationv1.FollowEdge {
	return &relationv1.FollowEdge{FollowerId: e.FollowerId, FolloweeId: e.FolloweeId, CreatedAt: e.CreatedAt}
}

func toPbBlock(e domain.BlockEdge) *relationv1.BlockEdge {
	return &relationv1.BlockEdge{Uid: e.Uid, BlockedUid: e.BlockedUid, CreatedAt: e.CreatedAt}
}
