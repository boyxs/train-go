package grpc

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	interactionv1 "github.com/webook/api/gen/interaction/v1"
	svcmocks "github.com/webook/internal/service/mocks"
)

func TestInteractionServer_GetHotBizIds_Happy(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockSvc := svcmocks.NewMockInteractionService(ctrl)
	mockSvc.EXPECT().
		ListHotBizIds(gomock.Any(), "article", 5).
		Return([]int64{3, 1, 2}, nil)

	conn := startBufServer(t, func(s *grpc.Server) {
		interactionv1.RegisterInteractionServiceServer(s, NewInteractionServer(mockSvc))
	})
	client := interactionv1.NewInteractionServiceClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := client.GetHotBizIds(ctx, &interactionv1.GetHotBizIdsRequest{
		Biz: "article", Limit: 5,
	})
	require.NoError(t, err)
	assert.Equal(t, []int64{3, 1, 2}, resp.GetBizIds())
}

func TestInteractionServer_GetHotBizIds_InvalidBiz(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockSvc := svcmocks.NewMockInteractionService(ctrl)

	conn := startBufServer(t, func(s *grpc.Server) {
		interactionv1.RegisterInteractionServiceServer(s, NewInteractionServer(mockSvc))
	})
	client := interactionv1.NewInteractionServiceClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := client.GetHotBizIds(ctx, &interactionv1.GetHotBizIdsRequest{
		Biz: "", Limit: 5,
	})
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestInteractionServer_GetCollectedBizIds_Happy(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockSvc := svcmocks.NewMockInteractionService(ctrl)
	mockSvc.EXPECT().
		ListCollectedBizIds(gomock.Any(), int64(42), "article", 5).
		Return([]int64{8, 5, 1}, nil)

	conn := startBufServer(t, func(s *grpc.Server) {
		interactionv1.RegisterInteractionServiceServer(s, NewInteractionServer(mockSvc))
	})
	client := interactionv1.NewInteractionServiceClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := client.GetCollectedBizIds(ctx, &interactionv1.GetCollectedBizIdsRequest{
		Uid: 42, Biz: "article", Limit: 5,
	})
	require.NoError(t, err)
	assert.Equal(t, []int64{8, 5, 1}, resp.GetBizIds())
}

func TestInteractionServer_GetCollectedBizIds_InvalidUid(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockSvc := svcmocks.NewMockInteractionService(ctrl)

	conn := startBufServer(t, func(s *grpc.Server) {
		interactionv1.RegisterInteractionServiceServer(s, NewInteractionServer(mockSvc))
	})
	client := interactionv1.NewInteractionServiceClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := client.GetCollectedBizIds(ctx, &interactionv1.GetCollectedBizIdsRequest{
		Uid: 0, Biz: "article", Limit: 5,
	})
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}
