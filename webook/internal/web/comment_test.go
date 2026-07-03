package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"

	commentv1 "github.com/webook/api/gen/comment/v1"
	"github.com/webook/internal/consts"
	"github.com/webook/internal/domain"
	svcmocks "github.com/webook/internal/service/mocks"
	grpcmocks "github.com/webook/internal/web/grpcmocks"
	"github.com/webook/pkg/logger"
)

// serveComment 起一个临时 gin server，uid>0 时注入登录态（模拟 OptionalPaths 命中）
func serveComment(uid int64, client commentv1.CommentServiceClient, intrSvc *svcmocks.MockInteractionService, userSvc *svcmocks.MockUserService, path, body string) *httptest.ResponseRecorder {
	h := NewInternalCommentHandler(client, intrSvc, userSvc, logger.NewNopLogger())
	server := gin.New()
	if uid > 0 {
		server.Use(func(c *gin.Context) {
			c.Set(consts.UserKey, UserClaims{Userid: uid})
			c.Next()
		})
	}
	h.RegisterRoutes(server)
	req, _ := http.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	return rec
}

type pageResult struct {
	Code int `json:"code"`
	Data struct {
		List  []CommentVO `json:"list"`
		Total int64       `json:"total"`
	} `json:"data"`
}

// pbComment 评论者 uid = 100+id
func pbComment(id int64) *commentv1.Comment {
	return &commentv1.Comment{
		Id:        id,
		UserId:    100 + id,
		Content:   "c",
		CreatedAt: id,
	}
}

// 列表(new)：聚合 likeCnt + 已登录 liked + Total 来自 CountComment + core 解析 uid→昵称
func TestCommentHandler_List_New(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := grpcmocks.NewMockCommentServiceClient(ctrl)
	client.EXPECT().ListComments(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, in *commentv1.ListCommentsRequest, _ ...grpc.CallOption) (*commentv1.ListCommentsResponse, error) {
			assert.Equal(t, domain.BizArticle, in.Biz)
			assert.Equal(t, int64(7), in.BizId)
			assert.Equal(t, int32(2), in.Limit)
			return &commentv1.ListCommentsResponse{Comments: []*commentv1.Comment{pbComment(1), pbComment(2)}}, nil
		})
	client.EXPECT().CountComment(gomock.Any(), gomock.Any()).Return(&commentv1.CountCommentResponse{Count: 5}, nil)

	intrSvc := svcmocks.NewMockInteractionService(ctrl)
	intrSvc.EXPECT().FindByBizIds(gomock.Any(), domain.BizComment, []int64{1, 2}).
		Return(map[int64]domain.Interaction{1: {LikeCount: 3}, 2: {LikeCount: 7}}, nil)
	intrSvc.EXPECT().FindUserLiked(gomock.Any(), int64(42), domain.BizComment, []int64{1, 2}).
		Return(map[int64]bool{1: true}, nil)

	userSvc := svcmocks.NewMockUserService(ctrl)
	userSvc.EXPECT().FindByIds(gomock.Any(), gomock.Any()).
		Return(map[int64]domain.User{101: {Id: 101, Nickname: "张三"}, 102: {Id: 102, Nickname: "李四"}}, nil)

	rec := serveComment(42, client, intrSvc, userSvc, "/comment/list", `{"articleId":7,"sort":"new","limit":2}`)
	assert.Equal(t, http.StatusOK, rec.Code)

	var r pageResult
	assert.NoError(t, json.NewDecoder(rec.Body).Decode(&r))
	assert.Equal(t, int64(5), r.Data.Total)
	assert.Len(t, r.Data.List, 2)
	assert.Equal(t, int64(3), r.Data.List[0].LikeCnt)
	assert.True(t, r.Data.List[0].Liked)
	assert.Equal(t, int64(7), r.Data.List[1].LikeCnt)
	assert.False(t, r.Data.List[1].Liked)
	assert.Equal(t, int64(101), r.Data.List[0].User.Id)
	assert.Equal(t, "张三", r.Data.List[0].User.Name, "core 解析 uid→昵称")
}

// 列表：interaction 聚合失败 → 降级填零（likeCnt=0/liked=false），列表照常返回不 500
func TestCommentHandler_List_InteractionDown_Degrades(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := grpcmocks.NewMockCommentServiceClient(ctrl)
	client.EXPECT().ListComments(gomock.Any(), gomock.Any()).
		Return(&commentv1.ListCommentsResponse{Comments: []*commentv1.Comment{pbComment(1), pbComment(2)}}, nil)
	client.EXPECT().CountComment(gomock.Any(), gomock.Any()).Return(&commentv1.CountCommentResponse{Count: 2}, nil)

	intrSvc := svcmocks.NewMockInteractionService(ctrl)
	intrSvc.EXPECT().FindByBizIds(gomock.Any(), domain.BizComment, []int64{1, 2}).
		Return(nil, errors.New("interaction 不可用"))
	intrSvc.EXPECT().FindUserLiked(gomock.Any(), int64(42), domain.BizComment, []int64{1, 2}).
		Return(nil, errors.New("interaction 不可用"))

	userSvc := svcmocks.NewMockUserService(ctrl)
	userSvc.EXPECT().FindByIds(gomock.Any(), gomock.Any()).Return(map[int64]domain.User{}, nil)

	rec := serveComment(42, client, intrSvc, userSvc, "/comment/list", `{"articleId":7,"sort":"new","limit":2}`)
	assert.Equal(t, http.StatusOK, rec.Code, "互动聚合失败应降级不 500")

	var r pageResult
	assert.NoError(t, json.NewDecoder(rec.Body).Decode(&r))
	assert.Len(t, r.Data.List, 2)
	assert.Equal(t, int64(0), r.Data.List[0].LikeCnt, "降级填零")
	assert.False(t, r.Data.List[0].Liked, "降级填零")
}

// 列表(hot)：core 内存按 likeCnt 降序排，未登录 liked 全 false 且不调 FindUserLiked
func TestCommentHandler_List_Hot_LoggedOut(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := grpcmocks.NewMockCommentServiceClient(ctrl)
	client.EXPECT().ListComments(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, in *commentv1.ListCommentsRequest, _ ...grpc.CallOption) (*commentv1.ListCommentsResponse, error) {
			assert.Equal(t, int32(hotWindow), in.Limit, "hot 应按窗口拉一批再内存排序")
			return &commentv1.ListCommentsResponse{Comments: []*commentv1.Comment{pbComment(1), pbComment(2), pbComment(3)}}, nil
		})
	client.EXPECT().CountComment(gomock.Any(), gomock.Any()).Return(&commentv1.CountCommentResponse{Count: 3}, nil)

	intrSvc := svcmocks.NewMockInteractionService(ctrl)
	intrSvc.EXPECT().FindByBizIds(gomock.Any(), domain.BizComment, []int64{1, 2, 3}).
		Return(map[int64]domain.Interaction{1: {LikeCount: 5}, 2: {LikeCount: 1}, 3: {LikeCount: 9}}, nil)
	// 未登录：不应调 FindUserLiked（无 EXPECT，调了即 fail）

	userSvc := svcmocks.NewMockUserService(ctrl)
	userSvc.EXPECT().FindByIds(gomock.Any(), gomock.Any()).Return(map[int64]domain.User{}, nil)

	rec := serveComment(0, client, intrSvc, userSvc, "/comment/list", `{"articleId":7,"sort":"hot","limit":10}`)
	assert.Equal(t, http.StatusOK, rec.Code)

	var r pageResult
	assert.NoError(t, json.NewDecoder(rec.Body).Decode(&r))
	assert.Len(t, r.Data.List, 3)
	// 9 > 5 > 1
	assert.Equal(t, int64(3), r.Data.List[0].Id)
	assert.Equal(t, int64(1), r.Data.List[1].Id)
	assert.Equal(t, int64(2), r.Data.List[2].Id)
	assert.False(t, r.Data.List[0].Liked)
}

// 回复懒加载：GetReplies + 聚合
func TestCommentHandler_Replies(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := grpcmocks.NewMockCommentServiceClient(ctrl)
	client.EXPECT().GetReplies(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, in *commentv1.GetRepliesRequest, _ ...grpc.CallOption) (*commentv1.GetRepliesResponse, error) {
			assert.Equal(t, int64(88), in.RootId)
			return &commentv1.GetRepliesResponse{Replies: []*commentv1.Comment{pbComment(10)}}, nil
		})

	intrSvc := svcmocks.NewMockInteractionService(ctrl)
	intrSvc.EXPECT().FindByBizIds(gomock.Any(), domain.BizComment, []int64{10}).
		Return(map[int64]domain.Interaction{10: {LikeCount: 2}}, nil)

	userSvc := svcmocks.NewMockUserService(ctrl)
	userSvc.EXPECT().FindByIds(gomock.Any(), gomock.Any()).Return(map[int64]domain.User{}, nil)

	rec := serveComment(0, client, intrSvc, userSvc, "/comment/replies", `{"rootId":88,"limit":10}`)
	assert.Equal(t, http.StatusOK, rec.Code)

	var r pageResult
	assert.NoError(t, json.NewDecoder(rec.Body).Decode(&r))
	assert.Len(t, r.Data.List, 1)
	assert.Equal(t, int64(2), r.Data.List[0].LikeCnt)
}

// 发表：core 注入 biz/uid，透传 pid，返回 VO（含解析昵称）
func TestCommentHandler_Create(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := grpcmocks.NewMockCommentServiceClient(ctrl)
	client.EXPECT().CreateComment(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, in *commentv1.CreateCommentRequest, _ ...grpc.CallOption) (*commentv1.CreateCommentResponse, error) {
			assert.Equal(t, domain.BizArticle, in.Biz)
			assert.Equal(t, int64(7), in.BizId)
			assert.Equal(t, int64(42), in.UserId)
			assert.Equal(t, int64(88), in.Pid)
			assert.Equal(t, "hello", in.Content)
			return &commentv1.CreateCommentResponse{Comment: pbComment(123)}, nil
		})
	intrSvc := svcmocks.NewMockInteractionService(ctrl)
	userSvc := svcmocks.NewMockUserService(ctrl)
	userSvc.EXPECT().FindByIds(gomock.Any(), gomock.Any()).
		Return(map[int64]domain.User{223: {Id: 223, Nickname: "钱七"}}, nil)

	rec := serveComment(42, client, intrSvc, userSvc, "/comment/create", `{"articleId":7,"content":"hello","pid":88}`)
	assert.Equal(t, http.StatusOK, rec.Code)

	var r struct {
		Code int `json:"code"`
		Data struct {
			Comment CommentVO `json:"comment"`
		} `json:"data"`
	}
	assert.NoError(t, json.NewDecoder(rec.Body).Decode(&r))
	assert.Equal(t, int64(123), r.Data.Comment.Id)
	assert.Equal(t, "钱七", r.Data.Comment.User.Name)
}

// 删除：透传 id + 当前 uid（鉴权由 comment server 做；不聚合，故不调 userSvc）
func TestCommentHandler_Delete(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := grpcmocks.NewMockCommentServiceClient(ctrl)
	client.EXPECT().DeleteComment(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, in *commentv1.DeleteCommentRequest, _ ...grpc.CallOption) (*commentv1.DeleteCommentResponse, error) {
			assert.Equal(t, int64(55), in.Id)
			assert.Equal(t, int64(42), in.UserId)
			return &commentv1.DeleteCommentResponse{}, nil
		})
	intrSvc := svcmocks.NewMockInteractionService(ctrl)
	userSvc := svcmocks.NewMockUserService(ctrl)

	rec := serveComment(42, client, intrSvc, userSvc, "/comment/delete", `{"id":55}`)
	assert.Equal(t, http.StatusOK, rec.Code)
}
