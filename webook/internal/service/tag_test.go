package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	tagv1 "github.com/boyxs/train-go/webook/api/gen/tag/v1"
	"github.com/boyxs/train-go/webook/internal/service"
	grpcmocks "github.com/boyxs/train-go/webook/internal/web/grpcmocks"
	"github.com/boyxs/train-go/webook/pkg/logger"
)

// newTagSvc GRPCTagService 的 Detail/Follow/Unfollow 仅用 tagCli；
// search/reader/intr 这些路径不触及，传 nil（只 Recommend/TagArticles 才用）。
func newTagSvc(t *testing.T, ctrl *gomock.Controller) (service.TagService, *grpcmocks.MockTagServiceClient) {
	t.Helper()
	client := grpcmocks.NewMockTagServiceClient(ctrl)
	return service.NewGRPCTagService(client, nil, nil, nil, logger.NewNopLogger()), client
}

// Detail（登录）：并发聚合 Detail(含 follow/weekly 计数) + FollowStatus(isFollowing)
func TestGRPCTagService_Detail_LoggedIn(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc, client := newTagSvc(t, ctrl)

	client.EXPECT().Detail(gomock.Any(), gomock.Any()).
		Return(&tagv1.Tag{Name: "Go", Slug: "go", RefCount: 3, FollowCount: 7, WeeklyNewCount: 2}, nil)
	client.EXPECT().FollowStatus(gomock.Any(), gomock.Any()).
		Return(&tagv1.FollowStatusResponse{IsFollowing: true}, nil)

	tag, isFollowing, err := svc.Detail(context.Background(), "go", 42)
	assert.NoError(t, err)
	assert.True(t, isFollowing)
	assert.Equal(t, "Go", tag.Name)
	assert.Equal(t, int64(7), tag.FollowCount)
	assert.Equal(t, int64(2), tag.WeeklyNewCount)
}

// Detail（未登录）：viewerId<=0 → 不调 FollowStatus，isFollowing 恒 false
func TestGRPCTagService_Detail_LoggedOut_SkipsFollowStatus(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc, client := newTagSvc(t, ctrl)

	client.EXPECT().Detail(gomock.Any(), gomock.Any()).
		Return(&tagv1.Tag{Name: "Go", Slug: "go"}, nil)
	// 未登录：不应调 FollowStatus（无 EXPECT，调了即 fail）

	_, isFollowing, err := svc.Detail(context.Background(), "go", 0)
	assert.NoError(t, err)
	assert.False(t, isFollowing)
}

// Detail：FollowStatus 失败 → 关注态降级 false，详情照常返回不报错
func TestGRPCTagService_Detail_FollowStatusDown_Degrades(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc, client := newTagSvc(t, ctrl)

	client.EXPECT().Detail(gomock.Any(), gomock.Any()).
		Return(&tagv1.Tag{Name: "Go", Slug: "go", FollowCount: 5}, nil)
	client.EXPECT().FollowStatus(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("tag 服务不可用"))

	tag, isFollowing, err := svc.Detail(context.Background(), "go", 42)
	assert.NoError(t, err, "关注态失败应降级不报错")
	assert.False(t, isFollowing, "降级 false")
	assert.Equal(t, int64(5), tag.FollowCount, "详情照常返回")
}

// Detail：tag 本体查询失败 → 传播错误（非降级）
func TestGRPCTagService_Detail_TagError_Propagates(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc, client := newTagSvc(t, ctrl)

	client.EXPECT().Detail(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("not found"))
	// 并发的 FollowStatus 可能被调（viewerId>0）；其结果因 Detail 出错被丢弃，容忍 0/1 次
	client.EXPECT().FollowStatus(gomock.Any(), gomock.Any()).
		Return(&tagv1.FollowStatusResponse{IsFollowing: true}, nil).AnyTimes()

	_, _, err := svc.Detail(context.Background(), "nope", 42)
	assert.Error(t, err, "tag 本体失败应传播")
}

// Follow / Unfollow：透传 uid+slug，返回 changed + 翻转后关注数
func TestGRPCTagService_FollowUnfollow(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc, client := newTagSvc(t, ctrl)

	client.EXPECT().Follow(gomock.Any(), gomock.Any()).
		Return(&tagv1.FollowResponse{Changed: true, FollowerCount: 8}, nil)
	changed, cnt, err := svc.Follow(context.Background(), 42, "go")
	assert.NoError(t, err)
	assert.True(t, changed)
	assert.Equal(t, int64(8), cnt)

	client.EXPECT().Unfollow(gomock.Any(), gomock.Any()).
		Return(&tagv1.FollowResponse{Changed: true, FollowerCount: 7}, nil)
	changed, cnt, err = svc.Unfollow(context.Background(), 42, "go")
	assert.NoError(t, err)
	assert.True(t, changed)
	assert.Equal(t, int64(7), cnt)
}
