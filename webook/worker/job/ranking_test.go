package job

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	rankingv1 "github.com/webook/api/gen/ranking/v1"
)

// mockRankingJobClient 桩 core RankingJobService client：记调用 + 透传的 date，其余 RPC 由嵌入接口占位。
type mockRankingJobClient struct {
	rankingv1.RankingJobServiceClient
	recomputeDims   []rankingv1.Dimension
	recomputeDates  []string
	archiveCalls    int
	lastArchiveDate string
}

func (m *mockRankingJobClient) Recompute(_ context.Context, req *rankingv1.RecomputeRequest, _ ...grpc.CallOption) (*rankingv1.RecomputeResponse, error) {
	m.recomputeDims = append(m.recomputeDims, req.GetDimension())
	m.recomputeDates = append(m.recomputeDates, req.GetDate())
	return &rankingv1.RecomputeResponse{}, nil
}

func (m *mockRankingJobClient) Archive(_ context.Context, req *rankingv1.ArchiveRequest, _ ...grpc.CallOption) (*rankingv1.ArchiveResponse, error) {
	m.archiveCalls++
	m.lastArchiveDate = req.GetDate()
	return &rankingv1.ArchiveResponse{}, nil
}

// 验证 task 闭包派发：维度 + wrapper 注入的 date 透传到 gRPC 请求。
// 不经 wrapper（锁/指标/recover 由 pkg/cronx 自测覆盖），故 wrapper 传 nil。
func TestRankingJob_Recompute_DispatchesToCore(t *testing.T) {
	m := &mockRankingJobClient{}
	j := NewRankingJob(m, nil)

	require.NoError(t, j.recompute(rankingv1.Dimension_DIMENSION_HOT)(context.Background(), "2026-06-29"))
	require.NoError(t, j.recompute(rankingv1.Dimension_DIMENSION_BEST)(context.Background(), "2026-06-29"))

	assert.Equal(t, []rankingv1.Dimension{rankingv1.Dimension_DIMENSION_HOT, rankingv1.Dimension_DIMENSION_BEST}, m.recomputeDims)
	assert.Equal(t, []string{"2026-06-29", "2026-06-29"}, m.recomputeDates)
}

func TestRankingJob_Archive_DispatchesToCore(t *testing.T) {
	m := &mockRankingJobClient{}
	j := NewRankingJob(m, nil)

	require.NoError(t, j.archive(context.Background(), "2026-06-29"))
	assert.Equal(t, 1, m.archiveCalls)
	assert.Equal(t, "2026-06-29", m.lastArchiveDate)
}
