package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gitee.com/train-cloud/geektime-basic-go/internal/consts"
	"gitee.com/train-cloud/geektime-basic-go/internal/domain"
	"gitee.com/train-cloud/geektime-basic-go/internal/integration/setup"
	"gitee.com/train-cloud/geektime-basic-go/internal/repository/dao"
	myJwt "gitee.com/train-cloud/geektime-basic-go/internal/web/jwt"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"
)

type ArticleHandlerSuite struct {
	suite.Suite
	db     *gorm.DB
	server *gin.Engine
}

func (h *ArticleHandlerSuite) TearDownTest() {
	h.truncate("article", "published_article")
}

func (h *ArticleHandlerSuite) truncate(tables ...string) {
	for _, table := range tables {
		err := h.db.Exec("TRUNCATE TABLE " + table).Error
		assert.NoError(h.T(), err)
	}
}

func (h *ArticleHandlerSuite) SetupSuite() {
	db := setup.InitDB()
	//redis := setup.InitRedis()
	server := gin.Default()
	server.Use(func(ctx *gin.Context) {
		ctx.Set(consts.UserKey, myJwt.UserClaims{
			Userid: 1,
		})

	})
	hdl := setup.InitArticleHandler()
	hdl.RegisterRoutes(server)
	h.db = db
	h.server = server
}

func TestArticleHandler(t *testing.T) {
	suite.Run(t, &ArticleHandlerSuite{})
}

func (h *ArticleHandlerSuite) TestArticleHandler_Edit() {
	t := h.T()
	mockNow := time.Now().UTC()
	testCases := []struct {
		name       string
		before     func(t *testing.T)
		after      func(t *testing.T)
		article    domain.Article
		wantCode   int
		wantResult Result[int64]
	}{
		{
			name:   "新建帖子",
			before: func(t *testing.T) {},
			after: func(t *testing.T) {
				var article dao.Article
				err := h.db.Where("author_id = ?", 1).
					First(&article).Error
				assert.NoError(t, err)
				article.CreatedAt = mockNow
				article.UpdatedAt = mockNow
				assert.Equal(t, dao.Article{
					Id:        1,
					Title:     "我的标题",
					Content:   "我的内容",
					AuthorId:  1,
					Status:    1,
					CreatedAt: mockNow,
					UpdatedAt: mockNow,
				}, article)
			},
			article: domain.Article{
				Title:   "我的标题",
				Content: "我的内容",
			},
			wantCode: http.StatusOK,
			wantResult: Result[int64]{
				Data: 1,
			},
		},
		{
			name: "修改帖子",
			before: func(t *testing.T) {
				err := h.db.Create(&dao.Article{
					Id:        2,
					Title:     "我的标题",
					Content:   "我的内容",
					AuthorId:  1,
					Status:    2,
					CreatedAt: mockNow,
				}).Error
				assert.NoError(t, err)
			},
			after: func(t *testing.T) {
				var article dao.Article
				err := h.db.Where("id = ?", 2).
					First(&article).Error
				assert.NoError(t, err)
				article.CreatedAt = mockNow
				article.UpdatedAt = mockNow
				assert.Equal(t, dao.Article{
					Id:        2,
					Title:     "新的标题",
					Content:   "新的内容",
					AuthorId:  1,
					Status:    1,
					CreatedAt: mockNow,
					UpdatedAt: mockNow,
				}, article)
			},
			article: domain.Article{
				Id:      2,
				Title:   "新的标题",
				Content: "新的内容",
			},
			wantCode: http.StatusOK,
			wantResult: Result[int64]{
				Data: 2,
			},
		},
		{
			name: "修改他人帖子",
			before: func(t *testing.T) {
				//模拟他人的帖子
				err := h.db.Create(&dao.Article{
					Id:        3,
					Title:     "他人的标题",
					Content:   "他人的内容",
					AuthorId:  9,
					Status:    2,
					CreatedAt: mockNow,
				}).Error
				assert.NoError(t, err)
			},
			after: func(t *testing.T) {
				var article dao.Article
				err := h.db.Where("id = ?", 3).
					First(&article).Error
				assert.NoError(t, err)
				article.CreatedAt = mockNow
				article.UpdatedAt = mockNow
				assert.Equal(t, dao.Article{
					Id:        3,
					Title:     "他人的标题",
					Content:   "他人的内容",
					AuthorId:  9,
					Status:    2,
					CreatedAt: mockNow,
					UpdatedAt: mockNow,
				}, article)
			},
			article: domain.Article{
				Id:      3,
				Title:   "新的标题",
				Content: "新的内容",
			},
			wantCode: http.StatusOK,
			wantResult: Result[int64]{
				Msg: "系统错误",
			},
		},
		{
			name:   "修改不存在的帖子",
			before: func(t *testing.T) {},
			after: func(t *testing.T) {
				var article dao.Article
				err := h.db.Where("id = ?", 999).First(&article).Error
				assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
			},
			article: domain.Article{
				Id:      999,
				Title:   "不存在的标题",
				Content: "不存在的内容",
			},
			wantCode: http.StatusOK,
			wantResult: Result[int64]{
				Msg: "系统错误",
			},
		},
		{
			name:   "编辑时标题为空",
			before: func(t *testing.T) {},
			after:  func(t *testing.T) {},
			article: domain.Article{
				Title:   "",
				Content: "有内容",
			},
			wantCode: http.StatusOK,
			wantResult: Result[int64]{
				Code: 4,
				Msg:  "标题和内容不能为空",
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.before(t)
			defer tc.after(t)

			body, err := json.Marshal(tc.article)
			assert.NoError(t, err)
			req, err := http.NewRequest(http.MethodPost, "/article/edit", bytes.NewReader(body))
			req.Header.Add("Content-Type", "application/json")
			assert.NoError(t, err)
			recorder := httptest.NewRecorder()

			h.server.ServeHTTP(recorder, req)

			assert.Equal(t, tc.wantCode, recorder.Code)
			if tc.wantCode != http.StatusOK {
				return
			}
			var result Result[int64]
			err = json.NewDecoder(recorder.Body).Decode(&result)
			assert.NoError(t, err)
			assert.Equal(t, tc.wantResult, result)
		})
	}
}

func (h *ArticleHandlerSuite) TestArticleHandler_Publish() {
	t := h.T()
	mockNow := time.Now().UTC()
	testCases := []struct {
		name       string
		before     func(t *testing.T)
		after      func(t *testing.T)
		req        string
		wantCode   int
		wantResult Result[any]
	}{
		{
			name:   "新建并发布",
			before: func(t *testing.T) {},
			after: func(t *testing.T) {
				// 验证制作库
				var article dao.Article
				err := h.db.Where("author_id = ?", 1).First(&article).Error
				assert.NoError(t, err)
				assert.Equal(t, "发布标题", article.Title)
				assert.Equal(t, "发布内容", article.Content)
				assert.Equal(t, uint8(domain.ArticleStatusPublished), article.Status)

				// 验证线上库
				var pub dao.PublishedArticle
				err = h.db.Where("author_id = ?", 1).First(&pub).Error
				assert.NoError(t, err)
				assert.Equal(t, article.Id, pub.Id)
				assert.Equal(t, "发布标题", pub.Title)
				assert.Equal(t, "发布内容", pub.Content)
				assert.Equal(t, uint8(domain.ArticleStatusPublished), pub.Status)
			},
			req:      `{"title":"发布标题","content":"发布内容"}`,
			wantCode: http.StatusOK,
			wantResult: Result[any]{
				Msg: "OK",
			},
		},
		{
			name: "修改已有文章并发布",
			before: func(t *testing.T) {
				err := h.db.Create(&dao.Article{
					Id:        10,
					Title:     "旧标题",
					Content:   "旧内容",
					AuthorId:  1,
					Status:    uint8(domain.ArticleStatusUnpublished),
					CreatedAt: mockNow,
				}).Error
				assert.NoError(t, err)
			},
			after: func(t *testing.T) {
				// 验证制作库已更新
				var article dao.Article
				err := h.db.Where("id = ?", 10).First(&article).Error
				assert.NoError(t, err)
				assert.Equal(t, "新标题", article.Title)
				assert.Equal(t, "新内容", article.Content)
				assert.Equal(t, uint8(domain.ArticleStatusPublished), article.Status)

				// 验证线上库
				var pub dao.PublishedArticle
				err = h.db.Where("id = ?", 10).First(&pub).Error
				assert.NoError(t, err)
				assert.Equal(t, "新标题", pub.Title)
				assert.Equal(t, "新内容", pub.Content)
				assert.Equal(t, uint8(domain.ArticleStatusPublished), pub.Status)
			},
			req:      `{"id":10,"title":"新标题","content":"新内容"}`,
			wantCode: http.StatusOK,
			wantResult: Result[any]{
				Msg: "OK",
			},
		},
		{
			name: "发布他人文章",
			before: func(t *testing.T) {
				err := h.db.Create(&dao.Article{
					Id:        20,
					Title:     "他人标题",
					Content:   "他人内容",
					AuthorId:  9,
					Status:    uint8(domain.ArticleStatusUnpublished),
					CreatedAt: mockNow,
				}).Error
				assert.NoError(t, err)
			},
			after: func(t *testing.T) {
				// 制作库数据不变
				var article dao.Article
				err := h.db.Where("id = ?", 20).First(&article).Error
				assert.NoError(t, err)
				assert.Equal(t, "他人标题", article.Title)
				assert.Equal(t, uint8(domain.ArticleStatusUnpublished), article.Status)

				// 线上库无数据
				var pub dao.PublishedArticle
				err = h.db.Where("id = ?", 20).First(&pub).Error
				assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
			},
			req:      `{"id":20,"title":"篡改标题","content":"篡改内容"}`,
			wantCode: http.StatusOK,
			wantResult: Result[any]{
				Msg: "系统错误",
			},
		},
		{
			name:   "发布不存在的文章",
			before: func(t *testing.T) {},
			after: func(t *testing.T) {
				// 制作库无数据
				var article dao.Article
				err := h.db.Where("id = ?", 999).First(&article).Error
				assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
				// 线上库无数据
				var pub dao.PublishedArticle
				err = h.db.Where("id = ?", 999).First(&pub).Error
				assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
			},
			req:      `{"id":999,"title":"不存在","content":"不存在内容"}`,
			wantCode: http.StatusOK,
			wantResult: Result[any]{
				Msg: "系统错误",
			},
		},
		{
			name:   "发布时标题为空",
			before: func(t *testing.T) {},
			after:  func(t *testing.T) {},
			req:    `{"title":"","content":"有内容"}`,
			wantCode: http.StatusOK,
			wantResult: Result[any]{
				Code: 4,
				Msg:  "标题和内容不能为空",
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.before(t)
			defer tc.after(t)

			req, err := http.NewRequest(http.MethodPost, "/article/publish",
				bytes.NewBufferString(tc.req))
			req.Header.Add("Content-Type", "application/json")
			assert.NoError(t, err)
			recorder := httptest.NewRecorder()

			h.server.ServeHTTP(recorder, req)

			assert.Equal(t, tc.wantCode, recorder.Code)
			var result Result[any]
			err = json.NewDecoder(recorder.Body).Decode(&result)
			assert.NoError(t, err)
			assert.Equal(t, tc.wantResult, result)
		})
	}
}

func (h *ArticleHandlerSuite) TestArticleHandler_Withdraw() {
	t := h.T()
	mockNow := time.Now().UTC()
	testCases := []struct {
		name       string
		before     func(t *testing.T)
		after      func(t *testing.T)
		req        string
		wantCode   int
		wantResult Result[any]
	}{
		{
			name: "撤回已发布文章",
			before: func(t *testing.T) {
				err := h.db.Create(&dao.Article{
					Id: 1, Title: "标题", Content: "内容",
					AuthorId: 1, Status: uint8(domain.ArticleStatusPublished),
					CreatedAt: mockNow,
				}).Error
				assert.NoError(t, err)
				err = h.db.Create(&dao.PublishedArticle{
					Id: 1, Title: "标题", Content: "内容",
					AuthorId: 1, Status: uint8(domain.ArticleStatusPublished),
					CreatedAt: mockNow,
				}).Error
				assert.NoError(t, err)
			},
			after: func(t *testing.T) {
				// 制作库 status=Private
				var article dao.Article
				err := h.db.Where("id = ?", 1).First(&article).Error
				assert.NoError(t, err)
				assert.Equal(t, uint8(domain.ArticleStatusPrivate), article.Status)
				// 线上库已删除
				var pub dao.PublishedArticle
				err = h.db.Where("id = ?", 1).First(&pub).Error
				assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
			},
			req:      `{"id":1}`,
			wantCode: http.StatusOK,
			wantResult: Result[any]{
				Msg: "OK",
			},
		},
		{
			name: "撤回他人文章",
			before: func(t *testing.T) {
				err := h.db.Create(&dao.Article{
					Id: 2, Title: "他人标题", Content: "他人内容",
					AuthorId: 9, Status: uint8(domain.ArticleStatusPublished),
					CreatedAt: mockNow,
				}).Error
				assert.NoError(t, err)
				err = h.db.Create(&dao.PublishedArticle{
					Id: 2, Title: "他人标题", Content: "他人内容",
					AuthorId: 9, Status: uint8(domain.ArticleStatusPublished),
					CreatedAt: mockNow,
				}).Error
				assert.NoError(t, err)
			},
			after: func(t *testing.T) {
				// 制作库不变
				var article dao.Article
				err := h.db.Where("id = ?", 2).First(&article).Error
				assert.NoError(t, err)
				assert.Equal(t, uint8(domain.ArticleStatusPublished), article.Status)
				// 线上库不变
				var pub dao.PublishedArticle
				err = h.db.Where("id = ?", 2).First(&pub).Error
				assert.NoError(t, err)
			},
			req:      `{"id":2}`,
			wantCode: http.StatusOK,
			wantResult: Result[any]{
				Msg: "系统错误",
			},
		},
		{
			name:   "撤回不存在的文章",
			before: func(t *testing.T) {},
			after: func(t *testing.T) {
				var pub dao.PublishedArticle
				err := h.db.Where("id = ?", 999).First(&pub).Error
				assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
			},
			req:      `{"id":999}`,
			wantCode: http.StatusOK,
			wantResult: Result[any]{
				Msg: "系统错误",
			},
		},
		{
			name: "撤回未发布草稿（幂等成功）",
			before: func(t *testing.T) {
				err := h.db.Create(&dao.Article{
					Id: 3, Title: "草稿", Content: "草稿内容",
					AuthorId: 1, Status: uint8(domain.ArticleStatusUnpublished),
					CreatedAt: mockNow,
				}).Error
				assert.NoError(t, err)
			},
			after: func(t *testing.T) {
				// 制作库 status 仍为 Unpublished（不变）
				var article dao.Article
				err := h.db.Where("id = ?", 3).First(&article).Error
				assert.NoError(t, err)
				assert.Equal(t, uint8(domain.ArticleStatusUnpublished), article.Status)
				// 线上库无记录
				var pub dao.PublishedArticle
				err = h.db.Where("id = ?", 3).First(&pub).Error
				assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
			},
			req:      `{"id":3}`,
			wantCode: http.StatusOK,
			wantResult: Result[any]{
				Msg: "OK",
			},
		},
		{
			name: "重复撤回（幂等成功）",
			before: func(t *testing.T) {
				err := h.db.Create(&dao.Article{
					Id: 4, Title: "已撤回", Content: "已撤回内容",
					AuthorId: 1, Status: uint8(domain.ArticleStatusPrivate),
					CreatedAt: mockNow,
				}).Error
				assert.NoError(t, err)
			},
			after: func(t *testing.T) {
				// 制作库 status 仍为 Private
				var article dao.Article
				err := h.db.Where("id = ?", 4).First(&article).Error
				assert.NoError(t, err)
				assert.Equal(t, uint8(domain.ArticleStatusPrivate), article.Status)
				// 线上库无记录
				var pub dao.PublishedArticle
				err = h.db.Where("id = ?", 4).First(&pub).Error
				assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
			},
			req:      `{"id":4}`,
			wantCode: http.StatusOK,
			wantResult: Result[any]{
				Msg: "OK",
			},
		},
		{
			name:   "id为零",
			before: func(t *testing.T) {},
			after:  func(t *testing.T) {},
			req:    `{"id":0}`,
			wantCode: http.StatusOK,
			wantResult: Result[any]{
				Msg: "系统错误",
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.before(t)
			defer tc.after(t)

			req, err := http.NewRequest(http.MethodPost, "/article/withdraw",
				bytes.NewBufferString(tc.req))
			req.Header.Add("Content-Type", "application/json")
			assert.NoError(t, err)
			recorder := httptest.NewRecorder()

			h.server.ServeHTTP(recorder, req)

			assert.Equal(t, tc.wantCode, recorder.Code)
			var result Result[any]
			err = json.NewDecoder(recorder.Body).Decode(&result)
			assert.NoError(t, err)
			assert.Equal(t, tc.wantResult, result)
		})
	}
}

type Result[T any] struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data T      `json:"data"`
}
