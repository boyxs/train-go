package cache

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/boyxs/train-go/webook/internal/domain"
	"github.com/boyxs/train-go/webook/internal/repository/cache/redismocks"
	"github.com/boyxs/train-go/webook/pkg/logger"
)

func TestRedisArticleCache_FirstPage(t *testing.T) {
	testCases := []struct {
		name      string
		mock      func(ctrl *gomock.Controller) redis.Cmdable
		articles  []domain.Article
		total     int64
		wantErr   error
		wantArts  []domain.Article
		wantTotal int64
	}{
		{
			name: "SetFirstPage后GetFirstPage正常读取",
			mock: func(ctrl *gomock.Controller) redis.Cmdable {
				cmd := redismocks.NewMockCmdable(ctrl)
				pageData := firstPageData{
					Articles: []domain.Article{
						{Id: 3, Title: "文章3"},
						{Id: 2, Title: "文章2"},
					},
					Total: 5,
				}
				data, _ := json.Marshal(pageData)

				// Set 期望
				setCmd := redis.NewStatusCmd(context.Background())
				setCmd.SetVal("OK")
				cmd.EXPECT().Set(gomock.Any(), "article:reader:first_page",
					data, gomock.Any()).Return(setCmd)

				// Get 期望
				getCmd := redis.NewStringCmd(context.Background())
				getCmd.SetVal(string(data))
				cmd.EXPECT().Get(gomock.Any(), "article:reader:first_page").
					Return(getCmd)

				return cmd
			},
			articles: []domain.Article{
				{Id: 3, Title: "文章3"},
				{Id: 2, Title: "文章2"},
			},
			total: 5,
			wantArts: []domain.Article{
				{Id: 3, Title: "文章3"},
				{Id: 2, Title: "文章2"},
			},
			wantTotal: 5,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			cmd := tc.mock(ctrl)
			c := NewRedisArticleCache(cmd, logger.NewNopLogger())
			ac := c.(*RedisArticleCache)

			ctx := context.Background()

			// Set
			err := ac.SetFirstPage(ctx, tc.articles, tc.total)
			assert.NoError(t, err)

			// Get
			arts, total, err := ac.GetFirstPage(ctx)
			assert.Equal(t, tc.wantErr, err)
			assert.Equal(t, tc.wantArts, arts)
			assert.Equal(t, tc.wantTotal, total)
		})
	}
}

func TestRedisArticleCache_GetFirstPage_Miss(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	cmd := redismocks.NewMockCmdable(ctrl)
	getCmd := redis.NewStringCmd(context.Background())
	getCmd.SetErr(redis.Nil)
	cmd.EXPECT().Get(gomock.Any(), "article:reader:first_page").Return(getCmd)

	c := NewRedisArticleCache(cmd, logger.NewNopLogger())
	ac := c.(*RedisArticleCache)

	_, _, err := ac.GetFirstPage(context.Background())
	assert.Equal(t, redis.Nil, err)
}

func TestRedisArticleCache_DelFirstPage(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	cmd := redismocks.NewMockCmdable(ctrl)
	delCmd := redis.NewIntCmd(context.Background())
	delCmd.SetVal(1)
	cmd.EXPECT().Del(gomock.Any(), "article:reader:first_page").Return(delCmd)

	c := NewRedisArticleCache(cmd, logger.NewNopLogger())
	ac := c.(*RedisArticleCache)

	err := ac.DelFirstPage(context.Background())
	assert.NoError(t, err)
}

// MGetPub 走 Get 流水线（集群安全）：命中返回、个别 miss/损坏跳过且不让整体报错。用 miniredis 跑真流水线。
func TestRedisArticleCache_MGetPub(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	c := NewRedisArticleCache(rdb, logger.NewNopLogger())
	ac := c.(*RedisArticleCache)
	ctx := context.Background()

	require.NoError(t, ac.SetPub(ctx, domain.Article{Id: 1, Title: "a"}))
	require.NoError(t, ac.SetPub(ctx, domain.Article{Id: 3, Title: "c"}))
	// id=2 不写（miss）；id=4 注入损坏 JSON
	require.NoError(t, rdb.Set(ctx, ac.getPubKey(4), "not-json", time.Minute).Err())

	got, err := ac.MGetPub(ctx, []int64{1, 2, 3, 4})
	require.NoError(t, err, "个别 miss/损坏不应让整体 MGetPub 报错")
	assert.Len(t, got, 2, "命中 1、3；2 miss、4 损坏均跳过")
	assert.Equal(t, "a", got[1].Title)
	assert.Equal(t, "c", got[3].Title)

	empty, err := ac.MGetPub(ctx, nil)
	require.NoError(t, err)
	assert.Empty(t, empty)
}
