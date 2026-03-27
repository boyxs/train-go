package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gitee.com/train-cloud/geektime-basic-go/internal/domain"
	"gitee.com/train-cloud/geektime-basic-go/internal/integration/setup"
	"gitee.com/train-cloud/geektime-basic-go/internal/repository/dao"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"
)

type ArticleReaderHandlerSuite struct {
	suite.Suite
	db     *gorm.DB
	server *gin.Engine
}

func (h *ArticleReaderHandlerSuite) TearDownTest() {
	h.truncate("article", "published_article")
}

func (h *ArticleReaderHandlerSuite) truncate(tables ...string) {
	for _, table := range tables {
		err := h.db.Exec("TRUNCATE TABLE " + table).Error
		assert.NoError(h.T(), err)
	}
}

func (h *ArticleReaderHandlerSuite) SetupSuite() {
	db := setup.InitDB()
	server := gin.Default()
	// 读者端不需要登录中间件
	hdl := setup.InitArticleReaderHandler()
	hdl.RegisterRoutes(server)
	h.db = db
	h.server = server
}

func TestArticleReaderHandler(t *testing.T) {
	suite.Run(t, &ArticleReaderHandlerSuite{})
}

func (h *ArticleReaderHandlerSuite) TestArticleReaderHandler_Page() {
	t := h.T()
	mockNow := time.Now().UTC()
	testCases := []struct {
		name       string
		before     func(t *testing.T)
		req        string
		wantCode   int
		wantResult Result[ArticleReaderListData]
	}{
		{
			name: "正常获取第一页",
			before: func(t *testing.T) {
				for i := int64(1); i <= 3; i++ {
					err := h.db.Create(&dao.PublishedArticle{
						Id: i, Title: "公开文章", Content: "内容",
						AuthorId: 1, Status: uint8(domain.ArticleStatusPublished),
						CreatedAt: mockNow,
					}).Error
					assert.NoError(t, err)
				}
			},
			req:      `{"page":1,"pageSize":2}`,
			wantCode: http.StatusOK,
			wantResult: Result[ArticleReaderListData]{
				Data: ArticleReaderListData{
					Total: 3,
					List: []ReaderArticleVO{
						{Id: 3, Title: "公开文章", AuthorId: 1},
						{Id: 2, Title: "公开文章", AuthorId: 1},
					},
				},
			},
		},
		{
			name:   "空列表",
			before: func(t *testing.T) {},
			req:    `{"page":1,"pageSize":10}`,
			wantCode: http.StatusOK,
			wantResult: Result[ArticleReaderListData]{
				Data: ArticleReaderListData{
					Total: 0,
					List:  []ReaderArticleVO{},
				},
			},
		},
		{
			name: "第二页",
			before: func(t *testing.T) {
				for i := int64(1); i <= 3; i++ {
					err := h.db.Create(&dao.PublishedArticle{
						Id: i, Title: "文章", Content: "内容",
						AuthorId: 1, Status: uint8(domain.ArticleStatusPublished),
						CreatedAt: mockNow,
					}).Error
					assert.NoError(t, err)
				}
			},
			req:      `{"page":2,"pageSize":2}`,
			wantCode: http.StatusOK,
			wantResult: Result[ArticleReaderListData]{
				Data: ArticleReaderListData{
					Total: 3,
					List: []ReaderArticleVO{
						{Id: 1, Title: "文章", AuthorId: 1},
					},
				},
			},
		},
		{
			name: "默认分页参数",
			before: func(t *testing.T) {
				err := h.db.Create(&dao.PublishedArticle{
					Id: 1, Title: "文章", Content: "内容",
					AuthorId: 1, Status: uint8(domain.ArticleStatusPublished),
					CreatedAt: mockNow,
				}).Error
				assert.NoError(t, err)
			},
			req:      `{}`,
			wantCode: http.StatusOK,
			wantResult: Result[ArticleReaderListData]{
				Data: ArticleReaderListData{
					Total: 1,
					List: []ReaderArticleVO{
						{Id: 1, Title: "文章", AuthorId: 1},
					},
				},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.before(t)
			defer h.truncate("published_article")

			req, err := http.NewRequest(http.MethodPost, "/article/reader/page",
				bytes.NewBufferString(tc.req))
			req.Header.Add("Content-Type", "application/json")
			assert.NoError(t, err)
			recorder := httptest.NewRecorder()

			h.server.ServeHTTP(recorder, req)

			assert.Equal(t, tc.wantCode, recorder.Code)
			var result Result[ArticleReaderListData]
			err = json.NewDecoder(recorder.Body).Decode(&result)
			assert.NoError(t, err)
			for i := range result.Data.List {
				result.Data.List[i].UpdatedAt = ""
			}
			assert.Equal(t, tc.wantResult, result)
		})
	}
}

func (h *ArticleReaderHandlerSuite) TestArticleReaderHandler_Detail() {
	t := h.T()
	mockNow := time.Now().UTC()
	testCases := []struct {
		name       string
		before     func(t *testing.T)
		req        string
		wantCode   int
		wantResult Result[domain.Article]
	}{
		{
			name: "正常获取已发布文章",
			before: func(t *testing.T) {
				err := h.db.Create(&dao.PublishedArticle{
					Id: 300, Title: "公开标题", Content: "公开内容",
					AuthorId: 1, Status: uint8(domain.ArticleStatusPublished),
					CreatedAt: mockNow,
				}).Error
				assert.NoError(t, err)
			},
			req:      `{"id":300}`,
			wantCode: http.StatusOK,
			wantResult: Result[domain.Article]{
				Data: domain.Article{
					Id:      300,
					Title:   "公开标题",
					Content: "公开内容",
					Author:  domain.Author{Id: 1},
					Status:  domain.ArticleStatusPublished,
				},
			},
		},
		{
			name:   "获取不存在的文章",
			before: func(t *testing.T) {},
			req:    `{"id":999}`,
			wantCode: http.StatusOK,
			wantResult: Result[domain.Article]{
				Msg: "文章不存在",
			},
		},
		{
			name:   "id为零",
			before: func(t *testing.T) {},
			req:    `{"id":0}`,
			wantCode: http.StatusOK,
			wantResult: Result[domain.Article]{
				Msg: "文章不存在",
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.before(t)
			defer h.truncate("published_article")

			req, err := http.NewRequest(http.MethodPost, "/article/reader/detail",
				bytes.NewBufferString(tc.req))
			req.Header.Add("Content-Type", "application/json")
			assert.NoError(t, err)
			recorder := httptest.NewRecorder()

			h.server.ServeHTTP(recorder, req)

			assert.Equal(t, tc.wantCode, recorder.Code)
			var result Result[domain.Article]
			err = json.NewDecoder(recorder.Body).Decode(&result)
			assert.NoError(t, err)
			result.Data.CreatedAt = time.Time{}
			result.Data.UpdatedAt = time.Time{}
			assert.Equal(t, tc.wantResult, result)
		})
	}
}

type ReaderArticleVO struct {
	Id        int64  `json:"id"`
	Title     string `json:"title"`
	AuthorId  int64  `json:"authorId"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

type ArticleReaderListData struct {
	List  []ReaderArticleVO `json:"list"`
	Total int64             `json:"total"`
}
