package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/boyxs/train-go/webook/internal/consts"
	"github.com/boyxs/train-go/webook/internal/domain"
	svcmocks "github.com/boyxs/train-go/webook/internal/service/mocks"
)

// serveComment 起临时 gin server，uid>0 时注入登录态（模拟 OptionalPaths 命中）。
// 接入层已瘦身：只 mock service.CommentService，验证「透传参数 + view→VO 映射」。
func serveComment(uid int64, svc *svcmocks.MockCommentService, path, body string) *httptest.ResponseRecorder {
	h := NewInternalCommentHandler(svc)
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

// List：透传 uid/articleId/sort/limit 到 service，将 view 树映射为 VO（含子回复）+ PageResult
func TestCommentHandler_List(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc := svcmocks.NewMockCommentService(ctrl)
	svc.EXPECT().List(gomock.Any(), int64(42), int64(7), "new", int32(0), int32(2)).
		Return([]domain.CommentView{
			{
				Id: 1, User: domain.CommentUser{Id: 101, Name: "张三"}, LikeCnt: 3, Liked: true,
				Children: []domain.CommentView{{Id: 11, User: domain.CommentUser{Id: 111, Name: "小明"}}},
			},
			{Id: 2, LikeCnt: 7},
		}, int64(5), nil)

	rec := serveComment(42, svc, "/comment/list", `{"articleId":7,"sort":"new","limit":2}`)
	assert.Equal(t, http.StatusOK, rec.Code)

	var r pageResult
	assert.NoError(t, json.NewDecoder(rec.Body).Decode(&r))
	assert.Equal(t, int64(5), r.Data.Total)
	assert.Len(t, r.Data.List, 2)
	assert.Equal(t, int64(3), r.Data.List[0].LikeCnt)
	assert.True(t, r.Data.List[0].Liked)
	assert.Equal(t, "张三", r.Data.List[0].User.Name)
	assert.Len(t, r.Data.List[0].Children, 1)
	assert.Equal(t, "小明", r.Data.List[0].Children[0].User.Name, "子回复也应映射")
}

// Create：透传 uid + req 字段到 service，返回映射后的 VO
func TestCommentHandler_Create(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc := svcmocks.NewMockCommentService(ctrl)
	svc.EXPECT().Create(gomock.Any(), int64(42), int64(7), "hello", int64(88)).
		Return(domain.CommentView{Id: 123, User: domain.CommentUser{Id: 223, Name: "钱七"}}, nil)

	rec := serveComment(42, svc, "/comment/create", `{"articleId":7,"content":"hello","pid":88}`)
	assert.Equal(t, http.StatusOK, rec.Code)

	var r struct {
		Data struct {
			Comment CommentVO `json:"comment"`
		} `json:"data"`
	}
	assert.NoError(t, json.NewDecoder(rec.Body).Decode(&r))
	assert.Equal(t, int64(123), r.Data.Comment.Id)
	assert.Equal(t, "钱七", r.Data.Comment.User.Name)
}

// Delete：透传 id + 当前 uid
func TestCommentHandler_Delete(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc := svcmocks.NewMockCommentService(ctrl)
	svc.EXPECT().Delete(gomock.Any(), int64(55), int64(42)).Return(nil)

	rec := serveComment(42, svc, "/comment/delete", `{"id":55}`)
	assert.Equal(t, http.StatusOK, rec.Code)
}
