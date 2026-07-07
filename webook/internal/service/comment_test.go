package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"

	commentv1 "github.com/webook/api/gen/comment/v1"
	"github.com/webook/internal/domain"
	"github.com/webook/internal/service"
	svcmocks "github.com/webook/internal/service/mocks"
	grpcmocks "github.com/webook/internal/web/grpcmocks"
	"github.com/webook/pkg/logger"
)

// pbComment 评论者 uid = 100+id
func pbComment(id int64) *commentv1.Comment {
	return &commentv1.Comment{Id: id, UserId: 100 + id, Content: "c", CreatedAt: id}
}

func newCommentSvc(t *testing.T, ctrl *gomock.Controller) (service.CommentService, *grpcmocks.MockCommentServiceClient, *svcmocks.MockInteractionService, *svcmocks.MockUserService) {
	t.Helper()
	client := grpcmocks.NewMockCommentServiceClient(ctrl)
	intrSvc := svcmocks.NewMockInteractionService(ctrl)
	userSvc := svcmocks.NewMockUserService(ctrl)
	return service.NewGRPCCommentService(client, intrSvc, userSvc, logger.NewNopLogger()), client, intrSvc, userSvc
}

// 列表(new)：聚合 likeCnt + 已登录 liked + Total 来自 CountComment + 解析 uid→昵称
func TestGRPCCommentService_List_New(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc, client, intrSvc, userSvc := newCommentSvc(t, ctrl)

	client.EXPECT().ListComments(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, in *commentv1.ListCommentsRequest, _ ...grpc.CallOption) (*commentv1.ListCommentsResponse, error) {
			assert.Equal(t, domain.BizArticle, in.Biz)
			assert.Equal(t, int64(7), in.BizId)
			assert.Equal(t, int32(2), in.Limit)
			return &commentv1.ListCommentsResponse{Comments: []*commentv1.Comment{pbComment(1), pbComment(2)}}, nil
		})
	client.EXPECT().CountComment(gomock.Any(), gomock.Any()).Return(&commentv1.CountCommentResponse{Count: 5}, nil)
	intrSvc.EXPECT().FindByBizIds(gomock.Any(), domain.BizComment, []int64{1, 2}).
		Return(map[int64]domain.Interaction{1: {LikeCount: 3}, 2: {LikeCount: 7}}, nil)
	intrSvc.EXPECT().FindUserLiked(gomock.Any(), int64(42), domain.BizComment, []int64{1, 2}).
		Return(map[int64]bool{1: true}, nil)
	userSvc.EXPECT().FindByIds(gomock.Any(), gomock.Any()).
		Return(map[int64]domain.User{101: {Id: 101, Nickname: "张三"}, 102: {Id: 102, Nickname: "李四"}}, nil)

	views, total, err := svc.List(context.Background(), 42, 7, "new", 0, 2)
	assert.NoError(t, err)
	assert.Equal(t, int64(5), total)
	assert.Len(t, views, 2)
	assert.Equal(t, int64(3), views[0].LikeCnt)
	assert.True(t, views[0].Liked)
	assert.Equal(t, int64(7), views[1].LikeCnt)
	assert.False(t, views[1].Liked)
	assert.Equal(t, int64(101), views[0].User.Id)
	assert.Equal(t, "张三", views[0].User.Name, "core 解析 uid→昵称")
}

// interaction 聚合失败 → 降级填零，列表照常返回不报错
func TestGRPCCommentService_List_InteractionDown_Degrades(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc, client, intrSvc, userSvc := newCommentSvc(t, ctrl)

	client.EXPECT().ListComments(gomock.Any(), gomock.Any()).
		Return(&commentv1.ListCommentsResponse{Comments: []*commentv1.Comment{pbComment(1), pbComment(2)}}, nil)
	client.EXPECT().CountComment(gomock.Any(), gomock.Any()).Return(&commentv1.CountCommentResponse{Count: 2}, nil)
	intrSvc.EXPECT().FindByBizIds(gomock.Any(), domain.BizComment, []int64{1, 2}).
		Return(nil, errors.New("interaction 不可用"))
	intrSvc.EXPECT().FindUserLiked(gomock.Any(), int64(42), domain.BizComment, []int64{1, 2}).
		Return(nil, errors.New("interaction 不可用"))
	userSvc.EXPECT().FindByIds(gomock.Any(), gomock.Any()).Return(map[int64]domain.User{}, nil)

	views, _, err := svc.List(context.Background(), 42, 7, "new", 0, 2)
	assert.NoError(t, err, "互动聚合失败应降级不报错")
	assert.Len(t, views, 2)
	assert.Equal(t, int64(0), views[0].LikeCnt, "降级填零")
	assert.False(t, views[0].Liked, "降级填零")
}

// hot：内存按 likeCnt 降序排，未登录 liked 全 false 且不调 FindUserLiked
func TestGRPCCommentService_List_Hot_LoggedOut(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc, client, intrSvc, userSvc := newCommentSvc(t, ctrl)

	client.EXPECT().ListComments(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, in *commentv1.ListCommentsRequest, _ ...grpc.CallOption) (*commentv1.ListCommentsResponse, error) {
			assert.Equal(t, int32(100), in.Limit, "hot 应按窗口(100)拉一批再内存排序")
			return &commentv1.ListCommentsResponse{Comments: []*commentv1.Comment{pbComment(1), pbComment(2), pbComment(3)}}, nil
		})
	client.EXPECT().CountComment(gomock.Any(), gomock.Any()).Return(&commentv1.CountCommentResponse{Count: 3}, nil)
	intrSvc.EXPECT().FindByBizIds(gomock.Any(), domain.BizComment, []int64{1, 2, 3}).
		Return(map[int64]domain.Interaction{1: {LikeCount: 5}, 2: {LikeCount: 1}, 3: {LikeCount: 9}}, nil)
	// 未登录：不应调 FindUserLiked（无 EXPECT，调了即 fail）
	userSvc.EXPECT().FindByIds(gomock.Any(), gomock.Any()).Return(map[int64]domain.User{}, nil)

	views, _, err := svc.List(context.Background(), 0, 7, "hot", 0, 10)
	assert.NoError(t, err)
	assert.Len(t, views, 3)
	assert.Equal(t, int64(3), views[0].Id) // 9 > 5 > 1
	assert.Equal(t, int64(1), views[1].Id)
	assert.Equal(t, int64(2), views[2].Id)
	assert.False(t, views[0].Liked)
}

// 回复懒加载：GetReplies + 聚合
func TestGRPCCommentService_Replies(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc, client, intrSvc, userSvc := newCommentSvc(t, ctrl)

	client.EXPECT().GetReplies(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, in *commentv1.GetRepliesRequest, _ ...grpc.CallOption) (*commentv1.GetRepliesResponse, error) {
			assert.Equal(t, int64(88), in.RootId)
			return &commentv1.GetRepliesResponse{Replies: []*commentv1.Comment{pbComment(10)}}, nil
		})
	intrSvc.EXPECT().FindByBizIds(gomock.Any(), domain.BizComment, []int64{10}).
		Return(map[int64]domain.Interaction{10: {LikeCount: 2}}, nil)
	userSvc.EXPECT().FindByIds(gomock.Any(), gomock.Any()).Return(map[int64]domain.User{}, nil)

	views, err := svc.Replies(context.Background(), 0, 88, 0, 10)
	assert.NoError(t, err)
	assert.Len(t, views, 1)
	assert.Equal(t, int64(2), views[0].LikeCnt)
}

// 发表：注入 biz/uid，透传 pid，返回含昵称的 view
func TestGRPCCommentService_Create(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc, client, _, userSvc := newCommentSvc(t, ctrl)

	client.EXPECT().CreateComment(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, in *commentv1.CreateCommentRequest, _ ...grpc.CallOption) (*commentv1.CreateCommentResponse, error) {
			assert.Equal(t, domain.BizArticle, in.Biz)
			assert.Equal(t, int64(7), in.BizId)
			assert.Equal(t, int64(42), in.UserId)
			assert.Equal(t, int64(88), in.Pid)
			assert.Equal(t, "hello", in.Content)
			return &commentv1.CreateCommentResponse{Comment: pbComment(123)}, nil
		})
	userSvc.EXPECT().FindByIds(gomock.Any(), gomock.Any()).
		Return(map[int64]domain.User{223: {Id: 223, Nickname: "钱七"}}, nil)

	v, err := svc.Create(context.Background(), 42, 7, "hello", 88)
	assert.NoError(t, err)
	assert.Equal(t, int64(123), v.Id)
	assert.Equal(t, "钱七", v.User.Name)
}

// 删除：透传 id + uid（鉴权由 comment server 做；不聚合，故不调 userSvc）
func TestGRPCCommentService_Delete(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc, client, _, _ := newCommentSvc(t, ctrl)

	client.EXPECT().DeleteComment(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, in *commentv1.DeleteCommentRequest, _ ...grpc.CallOption) (*commentv1.DeleteCommentResponse, error) {
			assert.Equal(t, int64(55), in.Id)
			assert.Equal(t, int64(42), in.UserId)
			return &commentv1.DeleteCommentResponse{}, nil
		})
	assert.NoError(t, svc.Delete(context.Background(), 55, 42))
}
