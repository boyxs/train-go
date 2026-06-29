package grpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	commentv1 "github.com/webook/api/gen/comment/v1"
	"github.com/webook/comment/domain"
	svcmocks "github.com/webook/comment/service/mocks"
)

func TestCommentServer_CreateComment(t *testing.T) {
	ctrl := gomock.NewController(t)
	svc := svcmocks.NewMockCommentService(ctrl)
	// 验证 req→domain 入参映射
	svc.EXPECT().Create(gomock.Any(), domain.Comment{
		Biz: "article", BizId: 1, UserId: 100, Content: "hi", Pid: 5,
	}).Return(domain.Comment{
		Id: 9, Biz: "article", BizId: 1, UserId: 100,
		Content: "hi", RootId: 2, Pid: 5, CreatedAt: 123,
	}, nil)

	resp, err := NewCommentServer(svc).CreateComment(context.Background(), &commentv1.CreateCommentRequest{
		Biz: "article", BizId: 1, UserId: 100, Content: "hi", Pid: 5,
	})
	require.NoError(t, err)
	// 验证 domain→pb 出参转换
	assert.Equal(t, int64(9), resp.Comment.GetId())
	assert.Equal(t, "hi", resp.Comment.GetContent())
	assert.Equal(t, int64(2), resp.Comment.GetRootId())
	assert.Equal(t, int64(5), resp.Comment.GetPid())
	assert.Equal(t, int64(100), resp.Comment.GetUserId())
}

func TestCommentServer_CreateComment_ValidatesBiz(t *testing.T) {
	ctrl := gomock.NewController(t)
	svc := svcmocks.NewMockCommentService(ctrl)
	// biz 空 → 参数校验拦截，不应调 svc
	_, err := NewCommentServer(svc).CreateComment(context.Background(), &commentv1.CreateCommentRequest{
		BizId: 1, UserId: 100, Content: "hi",
	})
	require.Error(t, err)
}

func TestCommentServer_ListComments_BatchConvert(t *testing.T) {
	ctrl := gomock.NewController(t)
	svc := svcmocks.NewMockCommentService(ctrl)
	// limit 缺省 → normLimit 兜底 20
	svc.EXPECT().List(gomock.Any(), "article", int64(1), 0, 20).Return([]domain.Comment{
		{Id: 1, Content: "a", UserId: 10},
		{Id: 2, Content: "b", UserId: 20},
	}, nil)

	resp, err := NewCommentServer(svc).ListComments(context.Background(), &commentv1.ListCommentsRequest{
		Biz: "article", BizId: 1,
	})
	require.NoError(t, err)
	require.Len(t, resp.Comments, 2)
	assert.Equal(t, "a", resp.Comments[0].GetContent())
	assert.Equal(t, int64(20), resp.Comments[1].GetUserId())
}

func TestCommentServer_DeleteComment(t *testing.T) {
	ctrl := gomock.NewController(t)
	svc := svcmocks.NewMockCommentService(ctrl)
	svc.EXPECT().Delete(gomock.Any(), int64(1), int64(100)).Return(nil)
	_, err := NewCommentServer(svc).DeleteComment(context.Background(), &commentv1.DeleteCommentRequest{Id: 1, UserId: 100})
	require.NoError(t, err)
}
