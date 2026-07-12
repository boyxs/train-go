package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/boyxs/train-go/webook/internal/domain"
	svcmocks "github.com/boyxs/train-go/webook/internal/service/mocks"
	"github.com/boyxs/train-go/webook/pkg/logger"
)

// serveSearch 起临时 gin server 发 POST /search/article（接入层无鉴权，纯搬运）。
// 只 mock service.ArticleSearchService，验证「空 query 挡 400 + 分页归一 + SearchResult→VO 映射」。
func serveSearch(svc *svcmocks.MockArticleSearchService, body string) *httptest.ResponseRecorder {
	h := NewInternalArticleSearchHandler(svc, logger.NewNopLogger())
	engine := gin.New()
	h.RegisterRoutes(engine)
	req, _ := http.NewRequest(http.MethodPost, "/search/article", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec
}

// 空 query（含纯空白）→ 400，且不调用 service。
func TestArticleSearchHandler_Search_EmptyQuery(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc := svcmocks.NewMockArticleSearchService(ctrl)
	// 无 EXPECT：调了 service 即 fail

	rec := serveSearch(svc, `{"query":"  "}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// 正常路径：命中 + facet → VO（list/total/facets）映射正确。
func TestArticleSearchHandler_Search_OK(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc := svcmocks.NewMockArticleSearchService(ctrl)
	svc.EXPECT().Search(gomock.Any(), "go", []string{"go"}, 1, 10).Return(domain.SearchResult{
		Articles: []domain.TaggedArticle{
			{
				Id:      1,
				Title:   "Go 并发",
				Author:  domain.Author{Id: 10, Name: "张三"},
				Tags:    []domain.Tag{{Slug: "go", Name: "Go"}},
				ReadCnt: 10, LikeCnt: 3,
			},
		},
		Total:  1,
		Facets: []domain.TagCount{{Slug: "go", Name: "Go", Count: 5}},
	}, nil)

	rec := serveSearch(svc, `{"query":"go","page":1,"size":10,"filter":{"tags":["go"]}}`)
	assert.Equal(t, http.StatusOK, rec.Code)

	var r struct {
		Data struct {
			List []struct {
				Id      int64  `json:"id"`
				Title   string `json:"title"`
				ReadCnt int64  `json:"readCnt"`
				Tags    []struct {
					Name string `json:"name"`
					Slug string `json:"slug"`
				} `json:"tags"`
			} `json:"list"`
			Total  int64 `json:"total"`
			Facets []struct {
				Name  string `json:"name"`
				Slug  string `json:"slug"`
				Count int64  `json:"count"`
			} `json:"facets"`
		} `json:"data"`
	}
	assert.NoError(t, json.NewDecoder(rec.Body).Decode(&r))
	assert.Equal(t, int64(1), r.Data.Total)
	if assert.Len(t, r.Data.List, 1) {
		assert.Equal(t, "Go 并发", r.Data.List[0].Title)
		assert.Equal(t, int64(10), r.Data.List[0].ReadCnt)
		if assert.Len(t, r.Data.List[0].Tags, 1) {
			assert.Equal(t, "Go", r.Data.List[0].Tags[0].Name)
		}
	}
	if assert.Len(t, r.Data.Facets, 1) {
		assert.Equal(t, "Go", r.Data.Facets[0].Name)
		assert.Equal(t, int64(5), r.Data.Facets[0].Count)
	}
}

// 分页归一：page 越界 → clamp maxPageIndex(10000)；size 超上限(>50) → 默认 10。经 mock 入参断言。
func TestArticleSearchHandler_Search_NormalizesPaging(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc := svcmocks.NewMockArticleSearchService(ctrl)
	svc.EXPECT().Search(gomock.Any(), "go", gomock.Any(), 10000, 10).Return(domain.SearchResult{}, nil)

	rec := serveSearch(svc, `{"query":"go","page":99999,"size":999}`)
	assert.Equal(t, http.StatusOK, rec.Code)
}

// service 报普通错误 → 框架转 500。
func TestArticleSearchHandler_Search_ServiceError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc := svcmocks.NewMockArticleSearchService(ctrl)
	svc.EXPECT().Search(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(domain.SearchResult{}, errors.New("boom"))

	rec := serveSearch(svc, `{"query":"go"}`)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}
