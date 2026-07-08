package web

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/boyxs/train-go/webook/internal/consts"
	"github.com/boyxs/train-go/webook/internal/domain"
	svcmocks "github.com/boyxs/train-go/webook/internal/service/mocks"
	"github.com/boyxs/train-go/webook/pkg/logger"
)

// serveInteraction 起临时 gin server，uid>0 时注入登录态（模拟 OptionalPaths/鉴权命中）。
func serveInteraction(uid int64, intrSvc *svcmocks.MockInteractionService, path, body string) *httptest.ResponseRecorder {
	h := NewInternalInteractionHandler(intrSvc, logger.NewNopLogger())
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

// 泛型点赞：biz=comment 透传到 svc.Like(biz="comment")
func TestInteractionHandler_Like_Comment(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	intrSvc := svcmocks.NewMockInteractionService(ctrl)
	intrSvc.EXPECT().Like(gomock.Any(), int64(42), domain.BizComment, int64(55)).Return(nil)

	rec := serveInteraction(42, intrSvc, "/interaction/like", `{"biz":"comment","bizId":55,"liked":true}`)
	assert.Equal(t, http.StatusOK, rec.Code)
}

// 泛型点赞：biz=article 仍走同一入口
func TestInteractionHandler_Like_Article(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	intrSvc := svcmocks.NewMockInteractionService(ctrl)
	intrSvc.EXPECT().Like(gomock.Any(), int64(42), domain.BizArticle, int64(7)).Return(nil)

	rec := serveInteraction(42, intrSvc, "/interaction/like", `{"biz":"article","bizId":7,"liked":true}`)
	assert.Equal(t, http.StatusOK, rec.Code)
}

// 取消点赞：liked=false 调 CancelLike
func TestInteractionHandler_Like_Cancel(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	intrSvc := svcmocks.NewMockInteractionService(ctrl)
	intrSvc.EXPECT().CancelLike(gomock.Any(), int64(42), domain.BizComment, int64(55)).Return(nil)

	rec := serveInteraction(42, intrSvc, "/interaction/like", `{"biz":"comment","bizId":55,"liked":false}`)
	assert.Equal(t, http.StatusOK, rec.Code)
}

// 非法 biz 被白名单拦截返回 400，且不触达 service（无 EXPECT，调了即 fail）
func TestInteractionHandler_Like_InvalidBiz(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	intrSvc := svcmocks.NewMockInteractionService(ctrl)

	rec := serveInteraction(42, intrSvc, "/interaction/like", `{"biz":"unknown","bizId":1,"liked":true}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
