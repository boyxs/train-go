package grpc

import (
	"context"

	"github.com/golang-module/carbon/v2"

	rankingv1 "github.com/boyxs/train-go/webook/api/gen/ranking/v1"
	"github.com/boyxs/train-go/webook/internal/service"
	"github.com/boyxs/train-go/webook/pkg/errs"
)

// RankingJobServer 把榜单重算/归档暴露给 webook-worker 调度器触发。
// 榜单数据与重算逻辑都在 core，worker 只按 cron 调本服务（对齐 bolee-task：调度器只触发，不持数据）。
type RankingJobServer struct {
	rankingv1.UnimplementedRankingJobServiceServer
	svc service.RankingService
}

func NewRankingJobServer(svc service.RankingService) *RankingJobServer {
	return &RankingJobServer{svc: svc}
}

func (s *RankingJobServer) Recompute(ctx context.Context, req *rankingv1.RecomputeRequest) (*rankingv1.RecomputeResponse, error) {
	date := req.GetDate()
	if date == "" {
		date = carbon.Now().ToDateString()
	}
	var err error
	switch req.GetDimension() {
	case rankingv1.Dimension_DIMENSION_HOT:
		err = s.svc.RecomputeHot(ctx, date)
	case rankingv1.Dimension_DIMENSION_BEST:
		err = s.svc.RecomputeBest(ctx, date)
	case rankingv1.Dimension_DIMENSION_NEW:
		err = s.svc.RecomputeNew(ctx, date)
	default:
		return nil, errs.New(400, "未知 ranking dimension: "+req.GetDimension().String())
	}
	if err != nil {
		return nil, err
	}
	return &rankingv1.RecomputeResponse{}, nil
}

func (s *RankingJobServer) Archive(ctx context.Context, req *rankingv1.ArchiveRequest) (*rankingv1.ArchiveResponse, error) {
	date := req.GetDate()
	if date == "" {
		date = carbon.Now().ToDateString()
	}
	if err := s.svc.Archive(ctx, date); err != nil {
		return nil, err
	}
	return &rankingv1.ArchiveResponse{}, nil
}
