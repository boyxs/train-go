package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	"github.com/webook/internal/consts"
	"github.com/webook/internal/domain"
	"github.com/webook/internal/integration/setup"
	"github.com/webook/internal/repository/dao"
	myJwt "github.com/webook/pkg/jwtx"
)

type ArticleAuthorHandlerSuite struct {
	suite.Suite
	db     *gorm.DB
	server *gin.Engine
}

func (h *ArticleAuthorHandlerSuite) TearDownTest() {
	h.truncate("article", "published_article", "interaction")
}

func (h *ArticleAuthorHandlerSuite) truncate(tables ...string) {
	for _, table := range tables {
		err := h.db.Exec("TRUNCATE TABLE " + table).Error
		assert.NoError(h.T(), err)
	}
}

func (h *ArticleAuthorHandlerSuite) SetupSuite() {
	db := setup.InitDB()
	//redis := setup.InitRedis()
	server := gin.Default()
	server.Use(func(ctx *gin.Context) {
		ctx.Set(consts.UserKey, myJwt.UserClaims{
			Userid: 1,
		})

	})
	hdl := setup.InitArticleAuthorHandler()
	hdl.RegisterRoutes(server)
	h.db = db
	h.server = server
}

func TestArticleAuthorHandler(t *testing.T) {
	suite.Run(t, &ArticleAuthorHandlerSuite{})
}

func (h *ArticleAuthorHandlerSuite) TestArticleAuthorHandler_Edit() {
	t := h.T()
	mockNow := time.Now().UnixMilli()
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
					Abstract:  "我的内容",
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
					Abstract:  "新的内容",
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
			// 越权写：DAO WHERE author_id 没匹配 → NotFound 透传 → HTTP 500
			wantCode: http.StatusNotFound,
			wantResult: Result[int64]{
				Code: 404,
				Msg:  "文章不存在或无权限",
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
			wantCode: http.StatusNotFound,
			wantResult: Result[int64]{
				Code: 404,
				Msg:  "文章不存在或无权限",
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
			// handler 返 Code=4 → ginx CodeToHttpStatus → HTTP 400
			wantCode: http.StatusBadRequest,
			wantResult: Result[int64]{
				Code: 400,
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

func (h *ArticleAuthorHandlerSuite) TestArticleAuthorHandler_Publish() {
	t := h.T()
	mockNow := time.Now().UnixMilli()
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
			req: `{"id":20,"title":"篡改标题","content":"篡改内容"}`,
			// 越权写 → NotFound 透传 → HTTP 500
			wantCode: http.StatusNotFound,
			wantResult: Result[any]{
				Code: 404,
				Msg:  "文章不存在或无权限",
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
			wantCode: http.StatusNotFound,
			wantResult: Result[any]{
				Code: 404,
				Msg:  "文章不存在或无权限",
			},
		},
		{
			name:   "发布时标题为空",
			before: func(t *testing.T) {},
			after:  func(t *testing.T) {},
			req:    `{"title":"","content":"有内容"}`,
			// handler 返 Code=4 → HTTP 400
			wantCode: http.StatusBadRequest,
			wantResult: Result[any]{
				Code: 400,
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

func (h *ArticleAuthorHandlerSuite) TestArticleAuthorHandler_Withdraw() {
	t := h.T()
	mockNow := time.Now().UnixMilli()
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
				Msg: "OK",
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
				Msg: "OK",
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
			name:     "id为零",
			before:   func(t *testing.T) {},
			after:    func(t *testing.T) {},
			req:      `{"id":0}`,
			wantCode: http.StatusOK,
			wantResult: Result[any]{
				Msg: "OK",
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

// TestArticleAuthorHandler_RepublishAfterWithdraw_ResetsDeletedAt
// 回归：published_article(_v1) 撤回（GORM softDelete:milli 设 deleted_at=now）后重新发布，
// Upsert 必须把 deleted_at 重置回 0，否则 FindById 因 GORM 自动注入 `WHERE deleted_at=0`
// 过滤而读不到行，业务侧表现为"重新发布完看不到文章"。
func (h *ArticleAuthorHandlerSuite) TestArticleAuthorHandler_RepublishAfterWithdraw_ResetsDeletedAt() {
	t := h.T()
	mockNow := time.Now().UnixMilli()

	// 1) 先建一条已发布文章
	require := assert.New(t)
	require.NoError(h.db.Create(&dao.Article{
		Id: 99030, Title: "v1", Content: "c1",
		AuthorId: 1, Status: uint8(domain.ArticleStatusPublished),
		CreatedAt: mockNow,
	}).Error)
	require.NoError(h.db.Create(&dao.PublishedArticle{
		Id: 99030, Title: "v1", Content: "c1",
		AuthorId: 1, Status: uint8(domain.ArticleStatusPublished),
		CreatedAt: mockNow,
	}).Error)

	// 2) 撤回 → published_article.deleted_at 应被 softDelete 设非 0
	wReq, err := http.NewRequest(http.MethodPost, "/article/withdraw",
		bytes.NewReader([]byte(`{"id":99030}`)))
	require.NoError(err)
	wReq.Header.Set("Content-Type", "application/json")
	wRec := httptest.NewRecorder()
	h.server.ServeHTTP(wRec, wReq)
	assert.Equal(t, http.StatusOK, wRec.Code)

	// 直接绕 GORM 软删过滤，读 deleted_at 原始值
	var deletedAtAfterWithdraw int64
	require.NoError(h.db.Raw(
		"SELECT deleted_at FROM published_article WHERE id = ?", 99030,
	).Scan(&deletedAtAfterWithdraw).Error)
	assert.Greater(t, deletedAtAfterWithdraw, int64(0),
		"撤回后 deleted_at 必须 > 0（GORM softDelete:milli 设当前时间戳）")

	// 3) 重新发布同 id → Upsert 走 ON DUPLICATE KEY UPDATE，
	//    本回归 case 期望 deleted_at 被列入 DoUpdates → 重置为 0
	pReq, err := http.NewRequest(http.MethodPost, "/article/publish",
		bytes.NewReader([]byte(`{"id":99030,"title":"v2","content":"c2"}`)))
	require.NoError(err)
	pReq.Header.Set("Content-Type", "application/json")
	pRec := httptest.NewRecorder()
	h.server.ServeHTTP(pRec, pReq)
	assert.Equal(t, http.StatusOK, pRec.Code)

	// 4) 验证 deleted_at 重置为 0 + title/content 已更新
	var deletedAtAfterRepublish int64
	require.NoError(h.db.Raw(
		"SELECT deleted_at FROM published_article WHERE id = ?", 99030,
	).Scan(&deletedAtAfterRepublish).Error)
	assert.Equal(t, int64(0), deletedAtAfterRepublish,
		"重新发布后 deleted_at 必须重置为 0，否则 FindById 因软删过滤读不到（bug 现场）")

	var pub dao.PublishedArticle
	// GORM First 自动注入 deleted_at=0，能 First 出来说明业务侧读路径无 ErrRecordNotFound
	require.NoError(h.db.Where("id = ?", 99030).First(&pub).Error)
	assert.Equal(t, "v2", pub.Title)
	assert.Equal(t, "c2", pub.Content)
}

func (h *ArticleAuthorHandlerSuite) TestArticleAuthorHandler_Detail() {
	t := h.T()
	mockNow := time.Now().UnixMilli()
	testCases := []struct {
		name       string
		before     func(t *testing.T)
		req        string
		wantCode   int
		wantResult Result[AuthorDetailVO]
	}{
		{
			name: "正常获取自己的文章",
			before: func(t *testing.T) {
				err := h.db.Create(&dao.Article{
					Id: 100, Title: "我的标题", Content: "我的内容",
					AuthorId: 1, Status: uint8(domain.ArticleStatusPublished),
					CreatedAt: mockNow,
				}).Error
				assert.NoError(t, err)
			},
			req:      `{"id":100}`,
			wantCode: http.StatusOK,
			wantResult: Result[AuthorDetailVO]{
				Data: AuthorDetailVO{
					Id:      100,
					Title:   "我的标题",
					Content: "我的内容",
					Status:  uint8(domain.ArticleStatusPublished),
					ReadCnt: 0,
				},
			},
		},
		{
			// handler errgroup 内 NotFound 透传 → ginx httpStatus → HTTP 500
			name:     "获取不存在的文章",
			before:   func(t *testing.T) {},
			req:      `{"id":999}`,
			wantCode: http.StatusNotFound,
			wantResult: Result[AuthorDetailVO]{
				Code: 404,
				Msg:  "文章不存在或无权限",
			},
		},
		{
			name: "获取他人文章",
			before: func(t *testing.T) {
				err := h.db.Create(&dao.Article{
					Id: 101, Title: "他人标题", Content: "他人内容",
					AuthorId: 9, Status: uint8(domain.ArticleStatusPublished),
					CreatedAt: mockNow,
				}).Error
				assert.NoError(t, err)
			},
			req:      `{"id":101}`,
			wantCode: http.StatusNotFound,
			wantResult: Result[AuthorDetailVO]{
				Code: 404,
				Msg:  "文章不存在或无权限",
			},
		},
		{
			name:     "id为零",
			before:   func(t *testing.T) {},
			req:      `{"id":0}`,
			wantCode: http.StatusNotFound,
			wantResult: Result[AuthorDetailVO]{
				Code: 404,
				Msg:  "文章不存在或无权限",
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.before(t)

			req, err := http.NewRequest(http.MethodPost, "/article/detail",
				bytes.NewBufferString(tc.req))
			req.Header.Add("Content-Type", "application/json")
			assert.NoError(t, err)
			recorder := httptest.NewRecorder()

			h.server.ServeHTTP(recorder, req)

			assert.Equal(t, tc.wantCode, recorder.Code)
			var result Result[AuthorDetailVO]
			err = json.NewDecoder(recorder.Body).Decode(&result)
			assert.NoError(t, err)
			// 忽略时间字段
			result.Data.CreatedAt = 0
			result.Data.UpdatedAt = 0
			assert.Equal(t, tc.wantResult, result)
		})
	}
}

func (h *ArticleAuthorHandlerSuite) TestArticleAuthorHandler_Page() {
	t := h.T()
	mockNow := time.Now().UnixMilli()
	testCases := []struct {
		name       string
		before     func(t *testing.T)
		req        string
		wantCode   int
		wantResult Result[ArticleListData]
	}{
		{
			name: "正常获取第一页",
			before: func(t *testing.T) {
				// 创建 3 篇文章
				for i := int64(1); i <= 3; i++ {
					err := h.db.Create(&dao.Article{
						Id: i, Title: "标题", Content: "内容",
						AuthorId: 1, Status: uint8(domain.ArticleStatusUnpublished),
						CreatedAt: mockNow,
					}).Error
					assert.NoError(t, err)
				}
			},
			req:      `{"page":1,"pageSize":2}`,
			wantCode: http.StatusOK,
			wantResult: Result[ArticleListData]{
				Data: ArticleListData{
					Total: 3,
					List: []ArticleVO{
						{Id: 3, Title: "标题", Status: uint8(domain.ArticleStatusUnpublished)},
						{Id: 2, Title: "标题", Status: uint8(domain.ArticleStatusUnpublished)},
					},
				},
			},
		},
		{
			name:     "空列表",
			before:   func(t *testing.T) {},
			req:      `{"page":1,"pageSize":10}`,
			wantCode: http.StatusOK,
			wantResult: Result[ArticleListData]{
				Data: ArticleListData{
					Total: 0,
					List:  []ArticleVO{},
				},
			},
		},
		{
			name: "第二页",
			before: func(t *testing.T) {
				for i := int64(1); i <= 3; i++ {
					err := h.db.Create(&dao.Article{
						Id: i, Title: "标题", Content: "内容",
						AuthorId: 1, Status: uint8(domain.ArticleStatusUnpublished),
						CreatedAt: mockNow,
					}).Error
					assert.NoError(t, err)
				}
			},
			req:      `{"page":2,"pageSize":2}`,
			wantCode: http.StatusOK,
			wantResult: Result[ArticleListData]{
				Data: ArticleListData{
					Total: 3,
					List: []ArticleVO{
						{Id: 1, Title: "标题", Status: uint8(domain.ArticleStatusUnpublished)},
					},
				},
			},
		},
		{
			name: "默认分页参数",
			before: func(t *testing.T) {
				err := h.db.Create(&dao.Article{
					Id: 1, Title: "标题", Content: "内容",
					AuthorId: 1, Status: uint8(domain.ArticleStatusUnpublished),
					CreatedAt: mockNow,
				}).Error
				assert.NoError(t, err)
			},
			req:      `{}`,
			wantCode: http.StatusOK,
			wantResult: Result[ArticleListData]{
				Data: ArticleListData{
					Total: 1,
					List: []ArticleVO{
						{Id: 1, Title: "标题", Status: uint8(domain.ArticleStatusUnpublished)},
					},
				},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.before(t)
			defer h.truncate("article", "published_article")

			req, err := http.NewRequest(http.MethodPost, "/article/page",
				bytes.NewBufferString(tc.req))
			req.Header.Add("Content-Type", "application/json")
			assert.NoError(t, err)
			recorder := httptest.NewRecorder()

			h.server.ServeHTTP(recorder, req)

			assert.Equal(t, tc.wantCode, recorder.Code)
			var result Result[ArticleListData]
			err = json.NewDecoder(recorder.Body).Decode(&result)
			assert.NoError(t, err)
			// 忽略动态时间字段
			for i := range result.Data.List {
				result.Data.List[i].UpdatedAt = 0
			}
			assert.Equal(t, tc.wantResult, result)
		})
	}
}

// ArticleVO 列表接口返回的简化文章结构（不含 Content）
type ArticleVO struct {
	Id        int64  `json:"id"`
	Title     string `json:"title"`
	Status    uint8  `json:"status"`
	ReadCnt   int64  `json:"readCnt"`
	UpdatedAt int64  `json:"updatedAt,omitempty"`
}

// AuthorDetailVO 作者视角文章详情
type AuthorDetailVO struct {
	Id        int64  `json:"id"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	Abstract  string `json:"abstract"`
	Status    uint8  `json:"status"`
	ReadCnt   int64  `json:"readCnt"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
}

type ArticleListData struct {
	List  []ArticleVO `json:"list"`
	Total int64       `json:"total"`
}

func (h *ArticleAuthorHandlerSuite) TestArticleAuthorHandler_List() {
	t := h.T()
	mockNow := time.Now().UnixMilli()
	testCases := []struct {
		name       string
		before     func(t *testing.T)
		wantCode   int
		wantResult Result[[]ArticleVO]
	}{
		{
			name: "返回全部文章（按id降序）",
			before: func(t *testing.T) {
				for i := int64(1); i <= 3; i++ {
					err := h.db.Create(&dao.Article{
						Id: i, Title: "标题", Content: "内容",
						AuthorId: 1, Status: uint8(domain.ArticleStatusUnpublished),
						CreatedAt: mockNow,
					}).Error
					assert.NoError(t, err)
				}
				// 他人文章不应出现
				err := h.db.Create(&dao.Article{
					Id: 99, Title: "他人", Content: "内容",
					AuthorId: 9, Status: uint8(domain.ArticleStatusPublished),
					CreatedAt: mockNow,
				}).Error
				assert.NoError(t, err)
			},
			wantCode: http.StatusOK,
			wantResult: Result[[]ArticleVO]{
				Data: []ArticleVO{
					{Id: 3, Title: "标题", Status: uint8(domain.ArticleStatusUnpublished)},
					{Id: 2, Title: "标题", Status: uint8(domain.ArticleStatusUnpublished)},
					{Id: 1, Title: "标题", Status: uint8(domain.ArticleStatusUnpublished)},
				},
			},
		},
		{
			name:     "无文章返回空数组",
			before:   func(t *testing.T) {},
			wantCode: http.StatusOK,
			wantResult: Result[[]ArticleVO]{
				Data: []ArticleVO{},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.before(t)
			defer h.truncate("article", "published_article")

			req, err := http.NewRequest(http.MethodPost, "/article/list",
				bytes.NewBufferString("{}"))
			req.Header.Add("Content-Type", "application/json")
			assert.NoError(t, err)
			recorder := httptest.NewRecorder()

			h.server.ServeHTTP(recorder, req)

			assert.Equal(t, tc.wantCode, recorder.Code)
			var result Result[[]ArticleVO]
			err = json.NewDecoder(recorder.Body).Decode(&result)
			assert.NoError(t, err)
			for i := range result.Data {
				result.Data[i].UpdatedAt = 0
			}
			assert.Equal(t, tc.wantResult, result)
		})
	}
}

func (h *ArticleAuthorHandlerSuite) TestArticleAuthorHandler_Delete() {
	t := h.T()
	mockNow := time.Now().UnixMilli()
	testCases := []struct {
		name       string
		before     func(t *testing.T)
		after      func(t *testing.T)
		req        string
		wantCode   int
		wantResult Result[any]
	}{
		{
			name: "删除自己的草稿文章",
			before: func(t *testing.T) {
				err := h.db.Create(&dao.Article{
					Id: 200, Title: "草稿", Content: "内容",
					AuthorId: 1, Status: uint8(domain.ArticleStatusUnpublished),
					CreatedAt: mockNow,
				}).Error
				assert.NoError(t, err)
			},
			after: func(t *testing.T) {
				// 制作库已删除
				var article dao.Article
				err := h.db.Where("id = ?", 200).First(&article).Error
				assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
			},
			req:      `{"id":200}`,
			wantCode: http.StatusOK,
			wantResult: Result[any]{
				Msg: "OK",
			},
		},
		{
			name: "删除自己的已发布文章",
			before: func(t *testing.T) {
				err := h.db.Create(&dao.Article{
					Id: 201, Title: "已发布", Content: "内容",
					AuthorId: 1, Status: uint8(domain.ArticleStatusPublished),
					CreatedAt: mockNow,
				}).Error
				assert.NoError(t, err)
				err = h.db.Create(&dao.PublishedArticle{
					Id: 201, Title: "已发布", Content: "内容",
					AuthorId: 1, Status: uint8(domain.ArticleStatusPublished),
					CreatedAt: mockNow,
				}).Error
				assert.NoError(t, err)
			},
			after: func(t *testing.T) {
				// 制作库已删除
				var article dao.Article
				err := h.db.Where("id = ?", 201).First(&article).Error
				assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
				// 线上库也删除
				var pub dao.PublishedArticle
				err = h.db.Where("id = ?", 201).First(&pub).Error
				assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
			},
			req:      `{"id":201}`,
			wantCode: http.StatusOK,
			wantResult: Result[any]{
				Msg: "OK",
			},
		},
		{
			// handler 把 NotFound（含越权）当 err 透传 → ginx httpStatus → HTTP 500
			name: "删除他人文章",
			before: func(t *testing.T) {
				err := h.db.Create(&dao.Article{
					Id: 202, Title: "他人文章", Content: "内容",
					AuthorId: 9, Status: uint8(domain.ArticleStatusPublished),
					CreatedAt: mockNow,
				}).Error
				assert.NoError(t, err)
			},
			after: func(t *testing.T) {
				// 制作库数据不变
				var article dao.Article
				err := h.db.Where("id = ?", 202).First(&article).Error
				assert.NoError(t, err)
				assert.Equal(t, "他人文章", article.Title)
			},
			req:      `{"id":202}`,
			wantCode: http.StatusNotFound,
			wantResult: Result[any]{
				Code: 404,
				Msg:  "文章不存在或无权限",
			},
		},
		{
			name:     "删除不存在的文章",
			before:   func(t *testing.T) {},
			after:    func(t *testing.T) {},
			req:      `{"id":999}`,
			wantCode: http.StatusNotFound,
			wantResult: Result[any]{
				Code: 404,
				Msg:  "文章不存在或无权限",
			},
		},
		{
			name:     "id为零",
			before:   func(t *testing.T) {},
			after:    func(t *testing.T) {},
			req:      `{"id":0}`,
			wantCode: http.StatusNotFound,
			wantResult: Result[any]{
				Code: 404,
				Msg:  "文章不存在或无权限",
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.before(t)
			defer tc.after(t)

			req, err := http.NewRequest(http.MethodPost, "/article/delete",
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
