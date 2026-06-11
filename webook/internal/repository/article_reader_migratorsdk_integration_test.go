package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/webook/internal/domain"
	"github.com/webook/internal/migratorsdk"
	cachemocks "github.com/webook/internal/repository/cache/mocks"
	"github.com/webook/internal/repository/dao"
	daomocks "github.com/webook/internal/repository/dao/mocks"
	"github.com/webook/pkg/logger"
)

// 集成测试：miniredis 真 Redis + mock DAO；驱动 RedisSwitchReader / RedisDualWriter 真实现按 stage 切换行为。
// 不连真 MySQL — DAO 用 mock 验证调用次数；目标是验证 SDK 与 Repository 的拼接正确。
func TestCacheArticleReaderRepository_SDKIntegration(t *testing.T) {
	const taskName = "published_article_v1"
	const articleID int64 = 42
	mockArticle := domain.Article{
		Id: articleID, Title: "t", Content: "c", Abstract: "a",
		Author: domain.Author{Id: 1}, Status: domain.ArticleStatusPublished,
	}

	setup := func(t *testing.T) (
		*miniredis.Miniredis,
		redis.Cmdable,
		*daomocks.MockArticleReaderDAO,
		*daomocks.MockArticleReaderDAO,
		*cachemocks.MockArticleCache,
		ArticleReaderRepository,
		*gomock.Controller,
	) {
		t.Helper()
		mr := miniredis.RunT(t)
		cli := redis.NewClient(&redis.Options{Addr: mr.Addr()})
		ctrl := gomock.NewController(t)
		oldDAO := daomocks.NewMockArticleReaderDAO(ctrl)
		newDAO := daomocks.NewMockArticleReaderDAO(ctrl)
		c := cachemocks.NewMockArticleCache(ctrl)
		l := logger.NewNopLogger()
		repo := NewCacheArticleReaderRepository(
			oldDAO, dao.ArticleReaderNewDAO(newDAO), c,
			migratorsdk.NewRedisSwitchReader(cli, l),
			migratorsdk.NewRedisDualWriter(cli, nil, l),
			migratorsdk.TaskName(taskName),
			l,
		)
		return mr, cli, oldDAO, newDAO, c, repo, ctrl
	}

	t.Run("I1 stage 未设（默认 SRC_ONLY）→ Upsert 只写 OLD", func(t *testing.T) {
		_, _, oldDAO, _, c, repo, ctrl := setup(t)
		defer ctrl.Finish()
		oldDAO.EXPECT().Upsert(gomock.Any(), gomock.Any()).Return(nil)
		c.EXPECT().DelFirstPage(gomock.Any()).Return(nil)
		c.EXPECT().DelPub(gomock.Any(), articleID).Return(nil)
		require.NoError(t, repo.Upsert(context.Background(), mockArticle))
	})

	t.Run("I2 stage=SRC_FIRST gray=100 → Upsert 双写 OLD + NEW", func(t *testing.T) {
		mr, _, oldDAO, newDAO, c, repo, ctrl := setup(t)
		defer ctrl.Finish()
		require.NoError(t, mr.Set("migrator:stage:"+taskName, "SRC_FIRST"))
		require.NoError(t, mr.Set("migrator:gray:"+taskName, "100"))
		oldDAO.EXPECT().Upsert(gomock.Any(), gomock.Any()).Return(nil)
		newDAO.EXPECT().Upsert(gomock.Any(), gomock.Any()).Return(nil)
		c.EXPECT().DelFirstPage(gomock.Any()).Return(nil)
		c.EXPECT().DelPub(gomock.Any(), articleID).Return(nil)
		require.NoError(t, repo.Upsert(context.Background(), mockArticle))
	})

	t.Run("I3 stage=DST_FIRST → 严格双写，NEW 失败时业务报错", func(t *testing.T) {
		mr, _, oldDAO, newDAO, _, repo, ctrl := setup(t)
		defer ctrl.Finish()
		require.NoError(t, mr.Set("migrator:stage:"+taskName, "DST_FIRST"))
		oldDAO.EXPECT().Upsert(gomock.Any(), gomock.Any()).Return(nil)
		newDAO.EXPECT().Upsert(gomock.Any(), gomock.Any()).Return(errors.New("NEW down"))
		err := repo.Upsert(context.Background(), mockArticle)
		assert.ErrorContains(t, err, "NEW down")
	})

	t.Run("I4 stage=DST_ONLY → FindById 走 newDAO", func(t *testing.T) {
		mr, _, _, newDAO, c, repo, ctrl := setup(t)
		defer ctrl.Finish()
		require.NoError(t, mr.Set("migrator:stage:"+taskName, "DST_ONLY"))
		c.EXPECT().GetPub(gomock.Any(), articleID).Return(domain.Article{}, redis.Nil)
		newDAO.EXPECT().FindById(gomock.Any(), articleID).Return(dao.PublishedArticle{Id: articleID, Title: "new-side"}, nil)
		c.EXPECT().SetPub(gomock.Any(), gomock.Any()).Return(nil)
		a, err := repo.FindById(context.Background(), articleID)
		require.NoError(t, err)
		assert.Equal(t, "new-side", a.Title)
	})

	t.Run("I5 stage=SRC_FIRST gray=0 → FindById 仍走 OLD（灰度未开）", func(t *testing.T) {
		mr, _, oldDAO, _, c, repo, ctrl := setup(t)
		defer ctrl.Finish()
		require.NoError(t, mr.Set("migrator:stage:"+taskName, "SRC_FIRST"))
		require.NoError(t, mr.Set("migrator:gray:"+taskName, "0"))
		c.EXPECT().GetPub(gomock.Any(), articleID).Return(domain.Article{}, redis.Nil)
		oldDAO.EXPECT().FindById(gomock.Any(), articleID).Return(dao.PublishedArticle{Id: articleID, Title: "old-side"}, nil)
		c.EXPECT().SetPub(gomock.Any(), gomock.Any()).Return(nil)
		a, err := repo.FindById(context.Background(), articleID)
		require.NoError(t, err)
		assert.Equal(t, "old-side", a.Title)
	})
}
