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
	h.truncate("article")
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

type Result[T any] struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data T      `json:"data"`
}
