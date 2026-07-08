package grpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	rankingv1 "github.com/boyxs/train-go/webook/api/gen/ranking/v1"
	svcmocks "github.com/boyxs/train-go/webook/internal/service/mocks"
)

func TestRankingJobServer_Recompute_RoutesByDimension(t *testing.T) {
	cases := []struct {
		dim    rankingv1.Dimension
		expect func(*svcmocks.MockRankingService)
	}{
		{rankingv1.Dimension_DIMENSION_HOT, func(m *svcmocks.MockRankingService) { m.EXPECT().RecomputeHot(gomock.Any(), "2026-06-29").Return(nil) }},
		{rankingv1.Dimension_DIMENSION_BEST, func(m *svcmocks.MockRankingService) { m.EXPECT().RecomputeBest(gomock.Any(), "2026-06-29").Return(nil) }},
		{rankingv1.Dimension_DIMENSION_NEW, func(m *svcmocks.MockRankingService) { m.EXPECT().RecomputeNew(gomock.Any(), "2026-06-29").Return(nil) }},
	}
	for _, c := range cases {
		t.Run(c.dim.String(), func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			svc := svcmocks.NewMockRankingService(ctrl)
			c.expect(svc)
			srv := NewRankingJobServer(svc)
			_, err := srv.Recompute(context.Background(),
				&rankingv1.RecomputeRequest{Dimension: c.dim, Date: "2026-06-29"})
			require.NoError(t, err)
		})
	}
}

func TestRankingJobServer_Recompute_UnknownDimension(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc := svcmocks.NewMockRankingService(ctrl) // 不应调用任何 Recompute*
	srv := NewRankingJobServer(svc)
	_, err := srv.Recompute(context.Background(), &rankingv1.RecomputeRequest{Dimension: rankingv1.Dimension_DIMENSION_UNSPECIFIED, Date: "2026-06-29"})
	assert.Error(t, err)
}

func TestRankingJobServer_Archive(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc := svcmocks.NewMockRankingService(ctrl)
	svc.EXPECT().Archive(gomock.Any(), "2026-06-29").Return(nil)
	srv := NewRankingJobServer(svc)
	_, err := srv.Archive(context.Background(), &rankingv1.ArchiveRequest{Date: "2026-06-29"})
	require.NoError(t, err)
}
