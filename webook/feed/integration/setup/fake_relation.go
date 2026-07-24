package setup

import (
	"context"

	"google.golang.org/grpc"

	relationv1 "github.com/boyxs/train-go/webook/api/gen/relation/v1"
)

// FakeRelationClient 集成测试桩（固定场景）：
// 所有作者均为普通作者（粉丝 5 < 阈值）；任意作者的粉丝含用户 1001；任意用户关注作者 7。
// 不拉起真实 relation 服务（对齐 CLAUDE.md「跨服务依赖用 fake 桩」）。
type FakeRelationClient struct{}

func NewFakeRelationClient() relationv1.RelationServiceClient { return &FakeRelationClient{} }

func (f *FakeRelationClient) GetStats(_ context.Context, in *relationv1.GetStatsRequest, _ ...grpc.CallOption) (*relationv1.GetStatsResponse, error) {
	return &relationv1.GetStatsResponse{Stats: &relationv1.RelationStats{Uid: in.GetUid(), FollowerCnt: 5}}, nil
}

func (f *FakeRelationClient) BatchGetStats(_ context.Context, in *relationv1.BatchGetStatsRequest, _ ...grpc.CallOption) (*relationv1.BatchGetStatsResponse, error) {
	m := make(map[int64]*relationv1.RelationStats, len(in.GetUids()))
	for _, uid := range in.GetUids() {
		m[uid] = &relationv1.RelationStats{Uid: uid, FollowerCnt: 5}
	}
	return &relationv1.BatchGetStatsResponse{Stats: m}, nil
}

func (f *FakeRelationClient) ListFollowers(_ context.Context, in *relationv1.ListRequest, _ ...grpc.CallOption) (*relationv1.ListFollowResponse, error) {
	return &relationv1.ListFollowResponse{
		Edges: []*relationv1.FollowEdge{{FollowerId: 1001, FolloweeId: in.GetUid()}}, NextCursor: 0,
	}, nil
}

func (f *FakeRelationClient) ListFollowees(_ context.Context, in *relationv1.ListRequest, _ ...grpc.CallOption) (*relationv1.ListFollowResponse, error) {
	return &relationv1.ListFollowResponse{
		Edges: []*relationv1.FollowEdge{{FollowerId: in.GetUid(), FolloweeId: 7}}, NextCursor: 0,
	}, nil
}

// 以下方法 feed 不调用，返回零值桩以满足接口。
func (f *FakeRelationClient) Follow(_ context.Context, _ *relationv1.FollowRequest, _ ...grpc.CallOption) (*relationv1.FollowResponse, error) {
	return &relationv1.FollowResponse{}, nil
}
func (f *FakeRelationClient) Unfollow(_ context.Context, _ *relationv1.FollowRequest, _ ...grpc.CallOption) (*relationv1.FollowResponse, error) {
	return &relationv1.FollowResponse{}, nil
}
func (f *FakeRelationClient) Block(_ context.Context, _ *relationv1.BlockRequest, _ ...grpc.CallOption) (*relationv1.BlockResponse, error) {
	return &relationv1.BlockResponse{}, nil
}
func (f *FakeRelationClient) Unblock(_ context.Context, _ *relationv1.BlockRequest, _ ...grpc.CallOption) (*relationv1.BlockResponse, error) {
	return &relationv1.BlockResponse{}, nil
}
func (f *FakeRelationClient) GetRelation(_ context.Context, _ *relationv1.GetRelationRequest, _ ...grpc.CallOption) (*relationv1.GetRelationResponse, error) {
	return &relationv1.GetRelationResponse{}, nil
}
func (f *FakeRelationClient) ListBlocks(_ context.Context, _ *relationv1.ListRequest, _ ...grpc.CallOption) (*relationv1.ListBlockResponse, error) {
	return &relationv1.ListBlockResponse{}, nil
}
