package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/webook/internal/consts"
	"github.com/webook/internal/integration/setup"
	"github.com/webook/internal/web"
	myJwt "github.com/webook/internal/web/jwt"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"
)

type ArticlePolishSuite struct {
	suite.Suite
	db     *gorm.DB
	cmd    redis.Cmdable
	server *gin.Engine
}

func (s *ArticlePolishSuite) SetupSuite() {
	s.db = setup.InitDB()
	s.cmd = setup.InitRedis()
	server := gin.Default()
	server.Use(func(ctx *gin.Context) {
		ctx.Set(consts.UserKey, myJwt.UserClaims{Userid: 1})
	})
	hdl := setup.InitArticlePolishHandler()
	hdl.RegisterRoutes(server)
	s.server = server
}

func (s *ArticlePolishSuite) SetupTest() {
	// 清除限流 key，避免跨测试干扰
	ctx := context.Background()
	key := fmt.Sprintf(consts.PolishRateLimitPattern, 1)
	s.cmd.Del(ctx, key)
}

func TestArticlePolish(t *testing.T) {
	suite.Run(t, &ArticlePolishSuite{})
}

func (s *ArticlePolishSuite) postJSON(path string, body string) *httptest.ResponseRecorder {
	req, _ := http.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	req.Header.Add("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	s.server.ServeHTTP(recorder, req)
	return recorder
}

func (s *ArticlePolishSuite) TestPolish_EmptyTitle() {
	t := s.T()
	recorder := s.postJSON("/article/polish", `{"title":"","content":"有内容"}`)
	assert.Equal(t, http.StatusOK, recorder.Code)
	var res web.Result
	err := json.NewDecoder(recorder.Body).Decode(&res)
	assert.NoError(t, err)
	assert.Equal(t, 4, res.Code)
	assert.Contains(t, res.Msg, "标题")
}

func (s *ArticlePolishSuite) TestPolish_EmptyContent() {
	t := s.T()
	recorder := s.postJSON("/article/polish", `{"title":"标题","content":""}`)
	assert.Equal(t, http.StatusOK, recorder.Code)
	var res web.Result
	err := json.NewDecoder(recorder.Body).Decode(&res)
	assert.NoError(t, err)
	assert.Equal(t, 4, res.Code)
	assert.Contains(t, res.Msg, "内容")
}

func (s *ArticlePolishSuite) TestPolish_InvalidJSON() {
	t := s.T()
	recorder := s.postJSON("/article/polish", "not json")
	assert.Equal(t, http.StatusOK, recorder.Code)
	var res web.Result
	err := json.NewDecoder(recorder.Body).Decode(&res)
	assert.NoError(t, err)
	assert.Equal(t, 4, res.Code)
}
