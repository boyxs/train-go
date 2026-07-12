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
	"github.com/boyxs/train-go/webook/pkg/ginx"
	"github.com/boyxs/train-go/webook/pkg/logger"
)

// serveTag 起临时 gin server（tag 路由挂在 ginx.Router 上），uid>0 时注入登录态。
// 接入层已瘦身：只 mock service.TagService，验证「透传 uid/slug + 关注态/计数 → VO 映射」。
func serveTag(uid int64, svc *svcmocks.MockTagService, method, path, body string) *httptest.ResponseRecorder {
	h := NewInternalTagHandler(svc, logger.NewNopLogger())
	engine := gin.New()
	if uid > 0 {
		engine.Use(func(c *gin.Context) {
			c.Set(consts.UserKey, UserClaims{Userid: uid})
			c.Next()
		})
	}
	h.RegisterRoutes(ginx.NewRouter(engine, ginx.NewRouteRegistry()))
	req, _ := http.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec
}

func decodeFollow(t *testing.T, rec *httptest.ResponseRecorder) followVO {
	var r struct {
		Data followVO `json:"data"`
	}
	assert.NoError(t, json.NewDecoder(rec.Body).Decode(&r))
	return r.Data
}

// Follow：POST /tag/:slug/follow 透传 uid+slug，返回 {isFollowing:true, followCount}
func TestTagHandler_Follow(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc := svcmocks.NewMockTagService(ctrl)
	svc.EXPECT().Follow(gomock.Any(), int64(42), "go").Return(true, int64(7), nil)

	rec := serveTag(42, svc, http.MethodPost, "/tag/go/follow", "")
	assert.Equal(t, http.StatusOK, rec.Code)
	vo := decodeFollow(t, rec)
	assert.True(t, vo.IsFollowing)
	assert.Equal(t, int64(7), vo.FollowCount)
}

// Unfollow：DELETE /tag/:slug/follow 透传 uid+slug，返回 {isFollowing:false, followCount}
func TestTagHandler_Unfollow(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc := svcmocks.NewMockTagService(ctrl)
	svc.EXPECT().Unfollow(gomock.Any(), int64(42), "go").Return(true, int64(6), nil)

	rec := serveTag(42, svc, http.MethodDelete, "/tag/go/follow", "")
	assert.Equal(t, http.StatusOK, rec.Code)
	vo := decodeFollow(t, rec)
	assert.False(t, vo.IsFollowing)
	assert.Equal(t, int64(6), vo.FollowCount)
}

// Detail（登录）：透传 viewerId，VO 带 followCount + isFollowing
func TestTagHandler_Detail_LoggedIn(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc := svcmocks.NewMockTagService(ctrl)
	svc.EXPECT().Detail(gomock.Any(), "go", int64(42)).
		Return(domain.Tag{Name: "Go", Slug: "go", RefCount: 3, FollowCount: 7, WeeklyNewCount: 2}, true, nil)

	rec := serveTag(42, svc, http.MethodGet, "/tag/go", "")
	assert.Equal(t, http.StatusOK, rec.Code)
	var r struct {
		Data tagDetailVO `json:"data"`
	}
	assert.NoError(t, json.NewDecoder(rec.Body).Decode(&r))
	assert.Equal(t, "Go", r.Data.Name)
	assert.Equal(t, int64(7), r.Data.FollowCount)
	assert.Equal(t, int64(2), r.Data.WeeklyNewCount)
	assert.True(t, r.Data.IsFollowing)
}

// Detail（未登录）：viewerId=0，isFollowing 恒 false
func TestTagHandler_Detail_Anonymous(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc := svcmocks.NewMockTagService(ctrl)
	svc.EXPECT().Detail(gomock.Any(), "go", int64(0)).
		Return(domain.Tag{Name: "Go", Slug: "go", FollowCount: 7}, false, nil)

	rec := serveTag(0, svc, http.MethodGet, "/tag/go", "")
	assert.Equal(t, http.StatusOK, rec.Code)
	var r struct {
		Data tagDetailVO `json:"data"`
	}
	assert.NoError(t, json.NewDecoder(rec.Body).Decode(&r))
	assert.Equal(t, int64(7), r.Data.FollowCount)
	assert.False(t, r.Data.IsFollowing)
}
