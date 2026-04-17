package cache

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/webook/internal/domain"
	"github.com/webook/internal/repository/cache/redismocks"
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
			c := NewRedisArticleCache(cmd)
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

	c := NewRedisArticleCache(cmd)
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

	c := NewRedisArticleCache(cmd)
	ac := c.(*RedisArticleCache)

	err := ac.DelFirstPage(context.Background())
	assert.NoError(t, err)
}
