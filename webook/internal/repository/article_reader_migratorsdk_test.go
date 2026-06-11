package repository

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

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

// stubSwitchReader 单测用：决策函数可配置 fixed side。
type stubSwitchReader struct {
	Side migratorsdk.Side
	Err  error
}

func (s stubSwitchReader) ChooseSide(_ context.Context, _ string, _ int64) (migratorsdk.Side, error) {
	return s.Side, s.Err
}

// stubDualWriter 单测用：按 Sides 顺序调 fn；WriteFn 提供自定义路径（如阶段语义模拟）。
type stubDualWriter struct {
	Sides   []migratorsdk.Side
	WriteFn func(ctx context.Context, taskName string, fn func(side migratorsdk.Side) error) error
}

func (s *stubDualWriter) Write(ctx context.Context, taskName string, fn func(side migratorsdk.Side) error) error {
	if s.WriteFn != nil {
		return s.WriteFn(ctx, taskName, fn)
	}
	for _, side := range s.Sides {
		if err := fn(side); err != nil {
			return err
		}
	}
	return nil
}

const testTaskName = "published_article_v1"

func TestCacheArticleReaderRepository_MigratorSDK(t *testing.T) {
	mockArticle := domain.Article{
		Id: 42, Title: "t", Content: "c", Abstract: "a",
		Author: domain.Author{Id: 1}, Status: domain.ArticleStatusPublished,
	}

	t.Run("NoOp SDK → 仅写 OLD，NEW 0 次（业务等价旧行为）", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		oldDAO := daomocks.NewMockArticleReaderDAO(ctrl)
		newDAO := daomocks.NewMockArticleReaderDAO(ctrl)
		c := cachemocks.NewMockArticleCache(ctrl)
		oldDAO.EXPECT().Upsert(gomock.Any(), gomock.Any()).Return(nil)
		c.EXPECT().DelFirstPage(gomock.Any()).Return(nil)
		c.EXPECT().DelPub(gomock.Any(), int64(42)).Return(nil)
		// newDAO 应当 0 次调用

		repo := NewCacheArticleReaderRepository(
			oldDAO, dao.ArticleReaderNewDAO(newDAO), c,
			migratorsdk.NewNoOpSwitchReader(),
			migratorsdk.NewNoOpDualWriter(),
			testTaskName,
			logger.NewNopLogger(),
		)
		require.NoError(t, repo.Upsert(context.Background(), mockArticle))
	})

	t.Run("SRC_FIRST 双写：OLD 必成 + NEW 失败 → 业务 nil（仅 log）", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		oldDAO := daomocks.NewMockArticleReaderDAO(ctrl)
		newDAO := daomocks.NewMockArticleReaderDAO(ctrl)
		c := cachemocks.NewMockArticleCache(ctrl)
		oldDAO.EXPECT().Upsert(gomock.Any(), gomock.Any()).Return(nil)
		newDAO.EXPECT().Upsert(gomock.Any(), gomock.Any()).Return(errors.New("NEW down"))
		c.EXPECT().DelFirstPage(gomock.Any()).Return(nil)
		c.EXPECT().DelPub(gomock.Any(), int64(42)).Return(nil)

		dw := &stubDualWriter{Sides: []migratorsdk.Side{migratorsdk.SideOld, migratorsdk.SideNew}}
		// SRC_FIRST 阶段：NEW 失败不抛错（migratorsdk.RedisDualWriter 真实现里有兜底；stub 模拟）
		dw.WriteFn = func(_ context.Context, _ string, fn func(side migratorsdk.Side) error) error {
			if err := fn(migratorsdk.SideOld); err != nil {
				return err
			}
			_ = fn(migratorsdk.SideNew) // NEW 失败吞掉，行为同 RedisDualWriter
			return nil
		}

		repo := NewCacheArticleReaderRepository(
			oldDAO, dao.ArticleReaderNewDAO(newDAO), c,
			stubSwitchReader{Side: migratorsdk.SideOld},
			dw, testTaskName,
			logger.NewNopLogger(),
		)
		require.NoError(t, repo.Upsert(context.Background(), mockArticle))
	})

	t.Run("DST_FIRST 双写：OLD 成 + NEW 成 → 业务 nil", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		oldDAO := daomocks.NewMockArticleReaderDAO(ctrl)
		newDAO := daomocks.NewMockArticleReaderDAO(ctrl)
		c := cachemocks.NewMockArticleCache(ctrl)
		oldDAO.EXPECT().Upsert(gomock.Any(), gomock.Any()).Return(nil)
		newDAO.EXPECT().Upsert(gomock.Any(), gomock.Any()).Return(nil)
		c.EXPECT().DelFirstPage(gomock.Any()).Return(nil)
		c.EXPECT().DelPub(gomock.Any(), int64(42)).Return(nil)

		dw := &stubDualWriter{Sides: []migratorsdk.Side{migratorsdk.SideOld, migratorsdk.SideNew}}
		repo := NewCacheArticleReaderRepository(
			oldDAO, dao.ArticleReaderNewDAO(newDAO), c,
			stubSwitchReader{Side: migratorsdk.SideNew}, // DST_FIRST 时读侧
			dw, testTaskName,
			logger.NewNopLogger(),
		)
		require.NoError(t, repo.Upsert(context.Background(), mockArticle))
	})

	t.Run("DST_FIRST 双写：NEW 失败 → 业务报错", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		oldDAO := daomocks.NewMockArticleReaderDAO(ctrl)
		newDAO := daomocks.NewMockArticleReaderDAO(ctrl)
		c := cachemocks.NewMockArticleCache(ctrl)
		oldDAO.EXPECT().Upsert(gomock.Any(), gomock.Any()).Return(nil)
		newDAO.EXPECT().Upsert(gomock.Any(), gomock.Any()).Return(errors.New("NEW down"))
		// 失败时 cache 清理不走（业务报错提前 return）

		dw := &stubDualWriter{}
		dw.WriteFn = func(_ context.Context, _ string, fn func(side migratorsdk.Side) error) error {
			if err := fn(migratorsdk.SideOld); err != nil {
				return err
			}
			return fn(migratorsdk.SideNew) // DST_FIRST 严格双写：NEW 失败抛错
		}
		repo := NewCacheArticleReaderRepository(
			oldDAO, dao.ArticleReaderNewDAO(newDAO), c,
			stubSwitchReader{Side: migratorsdk.SideNew},
			dw, testTaskName,
			logger.NewNopLogger(),
		)
		err := repo.Upsert(context.Background(), mockArticle)
		assert.ErrorContains(t, err, "NEW down")
	})

	t.Run("FindById side=Old → 仅 oldDAO", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		oldDAO := daomocks.NewMockArticleReaderDAO(ctrl)
		newDAO := daomocks.NewMockArticleReaderDAO(ctrl)
		c := cachemocks.NewMockArticleCache(ctrl)
		c.EXPECT().GetPub(gomock.Any(), int64(42)).Return(domain.Article{}, redis.Nil)
		oldDAO.EXPECT().FindById(gomock.Any(), int64(42)).Return(dao.PublishedArticle{Id: 42, Title: "from-old"}, nil)
		c.EXPECT().SetPub(gomock.Any(), gomock.Any()).Return(nil)

		repo := NewCacheArticleReaderRepository(
			oldDAO, dao.ArticleReaderNewDAO(newDAO), c,
			stubSwitchReader{Side: migratorsdk.SideOld},
			migratorsdk.NewNoOpDualWriter(),
			testTaskName,
			logger.NewNopLogger(),
		)
		a, err := repo.FindById(context.Background(), 42)
		require.NoError(t, err)
		assert.Equal(t, "from-old", a.Title)
	})

	t.Run("FindById side=New → 仅 newDAO", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		oldDAO := daomocks.NewMockArticleReaderDAO(ctrl)
		newDAO := daomocks.NewMockArticleReaderDAO(ctrl)
		c := cachemocks.NewMockArticleCache(ctrl)
		c.EXPECT().GetPub(gomock.Any(), int64(42)).Return(domain.Article{}, redis.Nil)
		newDAO.EXPECT().FindById(gomock.Any(), int64(42)).Return(dao.PublishedArticle{Id: 42, Title: "from-new"}, nil)
		c.EXPECT().SetPub(gomock.Any(), gomock.Any()).Return(nil)

		repo := NewCacheArticleReaderRepository(
			oldDAO, dao.ArticleReaderNewDAO(newDAO), c,
			stubSwitchReader{Side: migratorsdk.SideNew},
			migratorsdk.NewNoOpDualWriter(),
			testTaskName,
			logger.NewNopLogger(),
		)
		a, err := repo.FindById(context.Background(), 42)
		require.NoError(t, err)
		assert.Equal(t, "from-new", a.Title)
	})

	t.Run("FindByIds 批量按 ids[0] 决策路由", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		oldDAO := daomocks.NewMockArticleReaderDAO(ctrl)
		newDAO := daomocks.NewMockArticleReaderDAO(ctrl)
		c := cachemocks.NewMockArticleCache(ctrl)
		c.EXPECT().MGetPub(gomock.Any(), []int64{10, 11}).Return(map[int64]domain.Article{}, nil)
		newDAO.EXPECT().FindByIds(gomock.Any(), []int64{10, 11}).Return([]dao.PublishedArticle{
			{Id: 10, Title: "a"}, {Id: 11, Title: "b"},
		}, nil)

		var chosenSideAsHashKey atomic.Int64
		sw := stubSwitchReaderHook(func(_ context.Context, _ string, hashKey int64) (migratorsdk.Side, error) {
			chosenSideAsHashKey.Store(hashKey)
			return migratorsdk.SideNew, nil
		})
		repo := NewCacheArticleReaderRepository(
			oldDAO, dao.ArticleReaderNewDAO(newDAO), c,
			sw, migratorsdk.NewNoOpDualWriter(),
			testTaskName,
			logger.NewNopLogger(),
		)
		got, err := repo.FindByIds(context.Background(), []int64{10, 11})
		require.NoError(t, err)
		assert.Len(t, got, 2)
		assert.Equal(t, int64(10), chosenSideAsHashKey.Load(), "hashKey 应为 ids[0]")
	})

	t.Run("Page 不切，始终走 oldDAO", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		oldDAO := daomocks.NewMockArticleReaderDAO(ctrl)
		newDAO := daomocks.NewMockArticleReaderDAO(ctrl)
		c := cachemocks.NewMockArticleCache(ctrl)
		c.EXPECT().GetFirstPage(gomock.Any()).Return(nil, int64(0), redis.Nil)
		oldDAO.EXPECT().Page(gomock.Any(), 0, 10).Return([]dao.PublishedArticle{{Id: 1}}, nil)
		oldDAO.EXPECT().Count(gomock.Any()).Return(int64(1), nil)
		c.EXPECT().SetFirstPage(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		// newDAO / sw 不应被调

		repo := NewCacheArticleReaderRepository(
			oldDAO, dao.ArticleReaderNewDAO(newDAO), c,
			stubSwitchReader{Side: migratorsdk.SideNew}, // 即便决策 NEW，Page 也走 OLD
			migratorsdk.NewNoOpDualWriter(),
			testTaskName,
			logger.NewNopLogger(),
		)
		arts, cnt, err := repo.Page(context.Background(), 0, 10)
		require.NoError(t, err)
		assert.Equal(t, int64(1), cnt)
		assert.Len(t, arts, 1)
	})

	t.Run("Delete 走 dw.Write", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		oldDAO := daomocks.NewMockArticleReaderDAO(ctrl)
		newDAO := daomocks.NewMockArticleReaderDAO(ctrl)
		c := cachemocks.NewMockArticleCache(ctrl)
		oldDAO.EXPECT().Delete(gomock.Any(), int64(42), int64(1)).Return(nil)
		newDAO.EXPECT().Delete(gomock.Any(), int64(42), int64(1)).Return(nil)
		c.EXPECT().DelFirstPage(gomock.Any()).Return(nil)
		c.EXPECT().DelPub(gomock.Any(), int64(42)).Return(nil)

		dw := &stubDualWriter{Sides: []migratorsdk.Side{migratorsdk.SideOld, migratorsdk.SideNew}}
		repo := NewCacheArticleReaderRepository(
			oldDAO, dao.ArticleReaderNewDAO(newDAO), c,
			stubSwitchReader{Side: migratorsdk.SideOld},
			dw, testTaskName,
			logger.NewNopLogger(),
		)
		require.NoError(t, repo.Delete(context.Background(), 42, 1))
	})
}

// stubSwitchReaderHook 接受 hook func 作为决策，方便断言 hashKey 入参。
type stubSwitchReaderHook func(ctx context.Context, taskName string, hashKey int64) (migratorsdk.Side, error)

func (s stubSwitchReaderHook) ChooseSide(ctx context.Context, taskName string, hashKey int64) (migratorsdk.Side, error) {
	return s(ctx, taskName, hashKey)
}
