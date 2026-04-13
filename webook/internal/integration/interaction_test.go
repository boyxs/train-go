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
	"gitee.com/train-cloud/geektime-basic-go/internal/web"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"
)

// testUid 测试用的固定登录 uid
const testUid int64 = 1

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
	// 测试登录态：固定注入 uid=1 的 UserClaims，模拟已登录用户
	server.Use(func(ctx *gin.Context) {
		ctx.Set(consts.UserKey, web.UserClaims{Userid: testUid})
		ctx.Next()
	})
	readerHdl := setup.InitArticleReaderHandler()
	readerHdl.RegisterRoutes(server)
	intrHdl := setup.InitInteractionHandler()
	intrHdl.RegisterRoutes(server)
	s.db = db
	s.cmd = cmd
	s.server = server
}

func (s *InteractionSuite) SetupTest() {
	s.truncate("published_article", "interaction", "user_interaction")
}

func (s *InteractionSuite) TearDownTest() {
	s.truncate("published_article", "interaction", "user_interaction")
}

func (s *InteractionSuite) truncate(tables ...string) {
	for _, table := range tables {
		err := s.db.Exec("TRUNCATE TABLE " + table).Error
		assert.NoError(s.T(), err)
	}
	ctx := context.Background()
	// 清理 Redis 中 interaction 和 article 缓存
	for _, pattern := range []string{"interaction:*", "article:*"} {
		keys, _ := s.cmd.Keys(ctx, pattern).Result()
		if len(keys) > 0 {
			s.cmd.Del(ctx, keys...)
		}
	}
}

// clearInteractionCache 清理指定文章的缓存
func (s *InteractionSuite) clearInteractionCache(bizIds ...int64) {
	ctx := context.Background()
	for _, id := range bizIds {
		s.cmd.Del(ctx, fmt.Sprintf(consts.InteractionPattern, "article", id))
	}
}

// clearUserStateCache 清理用户状态缓存
func (s *InteractionSuite) clearUserStateCache(uid int64, bizIds ...int64) {
	ctx := context.Background()
	for _, id := range bizIds {
		s.cmd.Del(ctx, fmt.Sprintf(consts.InteractionStatePattern, "article", id, uid))
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
			s.truncate("published_article", "interaction", "user_interaction")
			tc.before(t)

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
			s.truncate("published_article", "interaction", "user_interaction")
			tc.before(t)

			recorder := s.postJSON("/article/reader/detail", tc.req)
			assert.Equal(t, tc.wantCode, recorder.Code)

			var result Result[ReaderDetailVO]
			err := json.NewDecoder(recorder.Body).Decode(&result)
			assert.NoError(t, err)
			result.Data.CreatedAt = 0
			result.Data.UpdatedAt = 0
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
			s.truncate("published_article", "interaction", "user_interaction")
			tc.before(t)

			recorder := s.postJSON("/article/reader/page", tc.req)
			assert.Equal(t, tc.wantCode, recorder.Code)

			var result Result[ArticleReaderListData]
			err := json.NewDecoder(recorder.Body).Decode(&result)
			assert.NoError(t, err)
			// 忽略时间字段
			for i := range result.Data.List {
				result.Data.List[i].CreatedAt = 0
				result.Data.List[i].UpdatedAt = 0
			}
			assert.Equal(t, tc.wantResult, result)
		})
	}
}

// ── /interaction/state 独立接口 + 用户状态缓存测试 ────────────────────────

type userStateResp struct {
	Liked     bool `json:"liked"`
	Collected bool `json:"collected"`
}

func (s *InteractionSuite) TestInteraction_State() {
	t := s.T()
	now := time.Now().UnixMilli()

	testCases := []struct {
		name      string
		before    func(t *testing.T)
		bizId     int64
		wantLiked bool
		wantColl  bool
	}{
		{
			name:   "未互动返回 false",
			before: func(t *testing.T) {},
			bizId:  1001,
		},
		{
			name: "已点赞已收藏",
			before: func(t *testing.T) {
				err := s.db.Create(&dao.UserInteraction{
					UserId: testUid, Biz: "article", BizId: 1002,
					Liked: true, Collected: true,
					CreatedAt: now, UpdatedAt: now,
				}).Error
				assert.NoError(t, err)
			},
			bizId:     1002,
			wantLiked: true,
			wantColl:  true,
		},
		{
			name: "已点赞未收藏",
			before: func(t *testing.T) {
				err := s.db.Create(&dao.UserInteraction{
					UserId: testUid, Biz: "article", BizId: 1003,
					Liked: true, Collected: false,
					CreatedAt: now, UpdatedAt: now,
				}).Error
				assert.NoError(t, err)
			},
			bizId:     1003,
			wantLiked: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			s.truncate("published_article", "interaction", "user_interaction")
			tc.before(t)

			req := fmt.Sprintf(`{"articleId":%d}`, tc.bizId)
			recorder := s.postJSON("/interaction/state", req)
			assert.Equal(t, http.StatusOK, recorder.Code)

			var result Result[userStateResp]
			err := json.NewDecoder(recorder.Body).Decode(&result)
			assert.NoError(t, err)
			assert.Equal(t, 0, result.Code)
			assert.Equal(t, tc.wantLiked, result.Data.Liked)
			assert.Equal(t, tc.wantColl, result.Data.Collected)
		})
	}
}

func (s *InteractionSuite) TestInteraction_StateCacheAside() {
	t := s.T()
	now := time.Now().UnixMilli()
	const bizId int64 = 2001

	s.truncate("published_article", "interaction", "user_interaction")
	s.clearUserStateCache(testUid, bizId)

	// DB 预置已点赞
	err := s.db.Create(&dao.UserInteraction{
		UserId: testUid, Biz: "article", BizId: bizId,
		Liked: true, Collected: false,
		CreatedAt: now, UpdatedAt: now,
	}).Error
	assert.NoError(t, err)

	// 第一次请求：缓存 miss → 回源 DB → 回填缓存
	req := fmt.Sprintf(`{"articleId":%d}`, bizId)
	recorder := s.postJSON("/interaction/state", req)
	assert.Equal(t, http.StatusOK, recorder.Code)
	var r1 Result[userStateResp]
	_ = json.NewDecoder(recorder.Body).Decode(&r1)
	assert.True(t, r1.Data.Liked)

	// 验证缓存已回填
	ctx := context.Background()
	key := fmt.Sprintf(consts.InteractionStatePattern, "article", bizId, testUid)
	data, err := s.cmd.HGetAll(ctx, key).Result()
	assert.NoError(t, err)
	assert.Equal(t, "1", data["liked"])
	assert.Equal(t, "0", data["collected"])

	// 改 DB 但不清缓存：下次请求应走缓存（拿到旧值）
	err = s.db.Model(&dao.UserInteraction{}).
		Where("user_id = ? AND biz_id = ?", testUid, bizId).
		Update("liked", false).Error
	assert.NoError(t, err)

	recorder = s.postJSON("/interaction/state", req)
	var r2 Result[userStateResp]
	_ = json.NewDecoder(recorder.Body).Decode(&r2)
	assert.True(t, r2.Data.Liked, "缓存命中应返回旧值 true")
}

func (s *InteractionSuite) TestInteraction_StateCacheInvalidation() {
	t := s.T()
	const bizId int64 = 3001

	s.truncate("published_article", "interaction", "user_interaction")
	s.clearUserStateCache(testUid, bizId)
	s.clearInteractionCache(bizId)

	// 先通过 State 接口查一次，触发缓存回填（初始全 false）
	req := fmt.Sprintf(`{"articleId":%d}`, bizId)
	s.postJSON("/interaction/state", req)

	// 确认缓存已存在
	ctx := context.Background()
	stateKey := fmt.Sprintf(consts.InteractionStatePattern, "article", bizId, testUid)
	exists, _ := s.cmd.Exists(ctx, stateKey).Result()
	assert.Equal(t, int64(1), exists)

	// 点赞：写操作应清用户状态缓存
	likeReq := fmt.Sprintf(`{"articleId":%d,"liked":true}`, bizId)
	recorder := s.postJSON("/interaction/like", likeReq)
	assert.Equal(t, http.StatusOK, recorder.Code)

	// 验证缓存已清除
	exists, _ = s.cmd.Exists(ctx, stateKey).Result()
	assert.Equal(t, int64(0), exists, "点赞后用户状态缓存应被清除")

	// 再查一次 state：回源 DB 应拿到最新 liked=true
	recorder = s.postJSON("/interaction/state", req)
	var r Result[userStateResp]
	_ = json.NewDecoder(recorder.Body).Decode(&r)
	assert.True(t, r.Data.Liked)
}

func (s *InteractionSuite) TestInteraction_DetailNoUserState() {
	t := s.T()
	now := time.Now().UnixMilli()
	const bizId int64 = 4001

	s.truncate("published_article", "interaction", "user_interaction")
	s.clearInteractionCache(bizId)

	// DB 预置聚合 + 用户状态
	err := s.db.Create(&dao.Interaction{
		BizId: bizId, Biz: "article",
		ReadCount: 5, LikeCount: 3, CollectCount: 2,
		CreatedAt: now, UpdatedAt: now,
	}).Error
	assert.NoError(t, err)
	err = s.db.Create(&dao.UserInteraction{
		UserId: testUid, Biz: "article", BizId: bizId,
		Liked: true, Collected: true,
		CreatedAt: now, UpdatedAt: now,
	}).Error
	assert.NoError(t, err)

	// /interaction/detail 应只返回聚合计数，不含 liked/collected
	req := fmt.Sprintf(`{"articleId":%d}`, bizId)
	recorder := s.postJSON("/interaction/detail", req)
	assert.Equal(t, http.StatusOK, recorder.Code)

	var r Result[domain.Interaction]
	err = json.NewDecoder(recorder.Body).Decode(&r)
	assert.NoError(t, err)
	assert.Equal(t, int64(5), r.Data.ReadCount)
	assert.Equal(t, int64(3), r.Data.LikeCount)
	assert.Equal(t, int64(2), r.Data.CollectCount)
	// Detail 不应该返回用户状态（uid=0）
	assert.False(t, r.Data.Liked)
	assert.False(t, r.Data.Collected)
}
