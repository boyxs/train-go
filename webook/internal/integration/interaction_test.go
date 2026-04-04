package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gitee.com/train-cloud/geektime-basic-go/internal/consts"
	"gitee.com/train-cloud/geektime-basic-go/internal/domain"
	"gitee.com/train-cloud/geektime-basic-go/internal/integration/setup"
	"gitee.com/train-cloud/geektime-basic-go/internal/repository/dao"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"
)

type InteractionSuite struct {
	suite.Suite
	db     *gorm.DB
	cmd    redis.Cmdable
	server *gin.Engine
}

func (s *InteractionSuite) SetupSuite() {
	db := setup.InitDB()
	cmd := setup.InitRedis()
	server := gin.Default()
	readerHdl := setup.InitArticleReaderHandler()
	readerHdl.RegisterRoutes(server)
	intrHdl := setup.InitInteractionHandler()
	intrHdl.RegisterRoutes(server)
	s.db = db
	s.cmd = cmd
	s.server = server
}

func (s *InteractionSuite) TearDownTest() {
	s.truncate("published_article", "interaction", "user_interaction")
}

func (s *InteractionSuite) truncate(tables ...string) {
	for _, table := range tables {
		err := s.db.Exec("TRUNCATE TABLE " + table).Error
		assert.NoError(s.T(), err)
	}
	// 清理 Redis 中所有 interaction 缓存
	ctx := context.Background()
	keys, _ := s.cmd.Keys(ctx, "interaction:*").Result()
	if len(keys) > 0 {
		s.cmd.Del(ctx, keys...)
	}
}

// clearInteractionCache 清理指定文章的缓存
func (s *InteractionSuite) clearInteractionCache(bizIds ...int64) {
	ctx := context.Background()
	for _, id := range bizIds {
		s.cmd.Del(ctx, fmt.Sprintf(consts.InteractionPattern, "article", id))
	}
}

func TestInteraction(t *testing.T) {
	suite.Run(t, &InteractionSuite{})
}

func (s *InteractionSuite) postJSON(path string, body string) *httptest.ResponseRecorder {
	req, _ := http.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	req.Header.Add("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	s.server.ServeHTTP(recorder, req)
	return recorder
}

func (s *InteractionSuite) TestInteraction_ReadReport() {
	t := s.T()
	testCases := []struct {
		name      string
		before    func(t *testing.T)
		req       string
		viewTimes int // 上报次数
		wantCode  int
		after     func(t *testing.T) // 验证 DB
	}{
		{
			name:      "单次阅读上报",
			before:    func(t *testing.T) {},
			req:       `{"articleId":1}`,
			viewTimes: 1,
			wantCode:  http.StatusOK,
			after: func(t *testing.T) {
				var intr dao.Interaction
				err := s.db.Where("biz = 'article' AND biz_id = ?", 1).First(&intr).Error
				assert.NoError(t, err)
				assert.Equal(t, int64(1), intr.ReadCount)
			},
		},
		{
			name:      "多次阅读累加",
			before:    func(t *testing.T) {},
			req:       `{"articleId":2}`,
			viewTimes: 3,
			wantCode:  http.StatusOK,
			after: func(t *testing.T) {
				var intr dao.Interaction
				err := s.db.Where("biz = 'article' AND biz_id = ?", 2).First(&intr).Error
				assert.NoError(t, err)
				assert.Equal(t, int64(3), intr.ReadCount)
			},
		},
		{
			name: "已有计数基础上累加",
			before: func(t *testing.T) {
				now := time.Now().UnixMilli()
				err := s.db.Create(&dao.Interaction{
					BizId: 3, Biz: "article", ReadCount: 10,
					CreatedAt: now, UpdatedAt: now,
				}).Error
				assert.NoError(t, err)
			},
			req:       `{"articleId":3}`,
			viewTimes: 2,
			wantCode:  http.StatusOK,
			after: func(t *testing.T) {
				var intr dao.Interaction
				err := s.db.Where("biz = 'article' AND biz_id = ?", 3).First(&intr).Error
				assert.NoError(t, err)
				assert.Equal(t, int64(12), intr.ReadCount)
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.before(t)
			defer s.truncate("published_article", "interaction", "user_interaction")

			for i := 0; i < tc.viewTimes; i++ {
				recorder := s.postJSON("/interaction/view", tc.req)
				assert.Equal(t, tc.wantCode, recorder.Code)
			}
			tc.after(t)
		})
	}
}

func (s *InteractionSuite) TestInteraction_ReaderDetailReadCnt() {
	t := s.T()
	mockNow := time.Now().UnixMilli()
	testCases := []struct {
		name       string
		before     func(t *testing.T)
		req        string
		wantCode   int
		wantResult Result[ReaderDetailVO]
	}{
		{
			name: "上报后查详情返回正确阅读量",
			before: func(t *testing.T) {
				err := s.db.Create(&dao.PublishedArticle{
					Id: 100, Title: "测试文章", Content: "测试内容", Abstract: "测试内容",
					AuthorId: 1, Status: uint8(domain.ArticleStatusPublished),
					CreatedAt: mockNow,
				}).Error
				assert.NoError(t, err)
				// 上报 2 次阅读
				s.postJSON("/interaction/view", `{"articleId":100}`)
				s.postJSON("/interaction/view", `{"articleId":100}`)
			},
			req:      `{"id":100}`,
			wantCode: http.StatusOK,
			wantResult: Result[ReaderDetailVO]{
				Data: ReaderDetailVO{
					Id:       100,
					Title:    "测试文章",
					Content:  "测试内容",
					Abstract: "测试内容",
					AuthorId: 1,
					ReadCnt:  2,
				},
			},
		},
		{
			name: "无阅读记录返回0",
			before: func(t *testing.T) {
				err := s.db.Create(&dao.PublishedArticle{
					Id: 101, Title: "零阅读", Content: "内容", Abstract: "内容",
					AuthorId: 1, Status: uint8(domain.ArticleStatusPublished),
					CreatedAt: mockNow,
				}).Error
				assert.NoError(t, err)
			},
			req:      `{"id":101}`,
			wantCode: http.StatusOK,
			wantResult: Result[ReaderDetailVO]{
				Data: ReaderDetailVO{
					Id:       101,
					Title:    "零阅读",
					Content:  "内容",
					Abstract: "内容",
					AuthorId: 1,
					ReadCnt:  0,
				},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.before(t)
			defer s.truncate("published_article", "interaction", "user_interaction")

			recorder := s.postJSON("/article/reader/detail", tc.req)
			assert.Equal(t, tc.wantCode, recorder.Code)

			var result Result[ReaderDetailVO]
			err := json.NewDecoder(recorder.Body).Decode(&result)
			assert.NoError(t, err)
			result.Data.UpdatedAt = ""
			assert.Equal(t, tc.wantResult, result)
		})
	}
}

func (s *InteractionSuite) TestInteraction_ReaderPageReadCnt() {
	t := s.T()
	mockNow := time.Now().UnixMilli()
	testCases := []struct {
		name       string
		before     func(t *testing.T)
		req        string
		wantCode   int
		wantResult Result[ArticleReaderListData]
	}{
		{
			name: "列表返回预置阅读量",
			before: func(t *testing.T) {
				err := s.db.Create(&dao.PublishedArticle{
					Id: 200, Title: "文章A", Content: "内容A",
					AuthorId: 1, Status: uint8(domain.ArticleStatusPublished),
					CreatedAt: mockNow,
				}).Error
				assert.NoError(t, err)
				err = s.db.Create(&dao.Interaction{
					BizId: 200, Biz: "article", ReadCount: 42,
					CreatedAt: mockNow, UpdatedAt: mockNow,
				}).Error
				assert.NoError(t, err)
			},
			req:      `{"page":1,"pageSize":10}`,
			wantCode: http.StatusOK,
			wantResult: Result[ArticleReaderListData]{
				Data: ArticleReaderListData{
					Total: 1,
					List: []ReaderArticleVO{
						{Id: 200, Title: "文章A", AuthorId: 1, ReadCnt: 42},
					},
				},
			},
		},
		{
			name: "无互动数据返回0",
			before: func(t *testing.T) {
				err := s.db.Create(&dao.PublishedArticle{
					Id: 201, Title: "文章B", Content: "内容B",
					AuthorId: 1, Status: uint8(domain.ArticleStatusPublished),
					CreatedAt: mockNow,
				}).Error
				assert.NoError(t, err)
			},
			req:      `{"page":1,"pageSize":10}`,
			wantCode: http.StatusOK,
			wantResult: Result[ArticleReaderListData]{
				Data: ArticleReaderListData{
					Total: 1,
					List: []ReaderArticleVO{
						{Id: 201, Title: "文章B", AuthorId: 1, ReadCnt: 0},
					},
				},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.before(t)
			defer s.truncate("published_article", "interaction", "user_interaction")

			recorder := s.postJSON("/article/reader/page", tc.req)
			assert.Equal(t, tc.wantCode, recorder.Code)

			var result Result[ArticleReaderListData]
			err := json.NewDecoder(recorder.Body).Decode(&result)
			assert.NoError(t, err)
			// 忽略时间字段
			for i := range result.Data.List {
				result.Data.List[i].UpdatedAt = ""
			}
			assert.Equal(t, tc.wantResult, result)
		})
	}
}
