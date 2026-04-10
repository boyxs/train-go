package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"gitee.com/train-cloud/geektime-basic-go/internal/consts"
	"gitee.com/train-cloud/geektime-basic-go/internal/integration/setup"
	"gitee.com/train-cloud/geektime-basic-go/internal/repository/dao"
	"gitee.com/train-cloud/geektime-basic-go/internal/web"
	myJwt "gitee.com/train-cloud/geektime-basic-go/internal/web/jwt"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"
)

type ClickEventSuite struct {
	suite.Suite
	db     *gorm.DB
	cmd    redis.Cmdable
	server *gin.Engine
}

func (s *ClickEventSuite) SetupSuite() {
	db := setup.InitDB()
	cmd := setup.InitRedis()
	server := gin.Default()
	server.Use(func(ctx *gin.Context) {
		ctx.Set(consts.UserKey, myJwt.UserClaims{Userid: 1})
	})
	hdl := setup.InitClickEventHandler()
	hdl.RegisterRoutes(server)
	s.db = db
	s.cmd = cmd
	s.server = server
}

func (s *ClickEventSuite) TearDownTest() {
	s.db.Exec("TRUNCATE TABLE ai_click_events")
	ctx := context.Background()
	s.cmd.Del(ctx, consts.ClickEventDashboardKey)
}

func TestClickEvent(t *testing.T) {
	suite.Run(t, &ClickEventSuite{})
}

func (s *ClickEventSuite) postJSON(path string, body string) *httptest.ResponseRecorder {
	req, _ := http.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	req.Header.Add("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	s.server.ServeHTTP(recorder, req)
	return recorder
}

func (s *ClickEventSuite) TestClick_Success() {
	t := s.T()
	recorder := s.postJSON("/ai/click", `{"article_id":100,"conversation_id":10}`)
	assert.Equal(t, http.StatusOK, recorder.Code)

	var res web.Result
	err := json.NewDecoder(recorder.Body).Decode(&res)
	assert.NoError(t, err)
	assert.Equal(t, 0, res.Code)

	// 验证 DB
	var event dao.ClickEvent
	err = s.db.Where("user_id = ? AND article_id = ? AND conversation_id = ?", 1, 100, 10).First(&event).Error
	assert.NoError(t, err)
	assert.Equal(t, int64(1), event.UserId)
	assert.Equal(t, int64(100), event.ArticleId)
	assert.Equal(t, "ai_chat", event.Source)
}

func (s *ClickEventSuite) TestClick_Dedup() {
	t := s.T()

	// 第一次点击
	recorder := s.postJSON("/ai/click", `{"article_id":200,"conversation_id":20}`)
	assert.Equal(t, http.StatusOK, recorder.Code)

	// 同一用户+文章+对话 重复点击
	recorder = s.postJSON("/ai/click", `{"article_id":200,"conversation_id":20}`)
	assert.Equal(t, http.StatusOK, recorder.Code)

	// DB 只有一条
	var count int64
	s.db.Model(&dao.ClickEvent{}).
		Where("user_id = ? AND article_id = ? AND conversation_id = ?", 1, 200, 20).
		Count(&count)
	assert.Equal(t, int64(1), count)
}

func (s *ClickEventSuite) TestClick_InvalidParams() {
	t := s.T()
	testCases := []struct {
		name string
		body string
	}{
		{"article_id为0", `{"article_id":0,"conversation_id":10}`},
		{"conversation_id为0", `{"article_id":100,"conversation_id":0}`},
		{"空body", `{}`},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := s.postJSON("/ai/click", tc.body)
			assert.Equal(t, http.StatusOK, recorder.Code)
			var res web.Result
			err := json.NewDecoder(recorder.Body).Decode(&res)
			assert.NoError(t, err)
			assert.Equal(t, 4, res.Code)
		})
	}
}

func (s *ClickEventSuite) TestDashboard_Empty() {
	t := s.T()
	recorder := s.postJSON("/ai/dashboard", "")
	assert.Equal(t, http.StatusOK, recorder.Code)

	var res web.Result
	err := json.NewDecoder(recorder.Body).Decode(&res)
	assert.NoError(t, err)
	assert.Equal(t, 0, res.Code)
}

func (s *ClickEventSuite) TestDashboard_WithData() {
	t := s.T()

	// 先插入几条点击记录
	s.postJSON("/ai/click", `{"article_id":1,"conversation_id":1}`)
	s.postJSON("/ai/click", `{"article_id":2,"conversation_id":1}`)
	s.postJSON("/ai/click", `{"article_id":1,"conversation_id":2}`)

	// 清缓存确保查 DB
	s.cmd.Del(context.Background(), consts.ClickEventDashboardKey)

	recorder := s.postJSON("/ai/dashboard", "")
	assert.Equal(t, http.StatusOK, recorder.Code)

	var res web.Result
	err := json.NewDecoder(recorder.Body).Decode(&res)
	assert.NoError(t, err)
	assert.Equal(t, 0, res.Code)
	assert.NotNil(t, res.Data)

	// 解析 data 验证统计
	dataBytes, _ := json.Marshal(res.Data)
	var dashboard map[string]any
	err = json.Unmarshal(dataBytes, &dashboard)
	assert.NoError(t, err)
	assert.Equal(t, float64(3), dashboard["totalClicks"])
	assert.Equal(t, float64(1), dashboard["uniqueUsers"])
	assert.Equal(t, float64(2), dashboard["uniqueArticles"])
}
