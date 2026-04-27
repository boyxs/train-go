package cache

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"

	"github.com/webook/internal/domain"
	"github.com/webook/pkg/logger"
)

func newRankingCache(t *testing.T) (*miniredis.Miniredis, RankingCache) {
	t.Helper()
	mr := miniredis.RunT(t)
	cli := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return mr, NewRedisArticleRankingCache(cli, logger.NewNopLogger())
}

func TestRedisRankingCache_ReplaceAndTop(t *testing.T) {
	_, c := newRankingCache(t)
	ctx := context.Background()

	items := []domain.ArticleRanking{
		{ArticleId: 100, Score: 9832},
		{ArticleId: 200, Score: 7621},
		{ArticleId: 300, Score: 6890},
	}
	assert.NoError(t, c.ReplaceTop(ctx, "2026-04-21", "hot", "", items))

	got, err := c.Top(ctx, "2026-04-21", "hot", "", 10)
	assert.NoError(t, err)
	assert.Len(t, got, 3)
	assert.Equal(t, int64(100), got[0].ArticleId)
	assert.Equal(t, 1, got[0].Rank)
	assert.Equal(t, int64(300), got[2].ArticleId)
	assert.Equal(t, 3, got[2].Rank)
}

func TestRedisRankingCache_IncrScore(t *testing.T) {
	_, c := newRankingCache(t)
	ctx := context.Background()

	assert.NoError(t, c.IncrScore(ctx, "2026-04-21", "hot", "", 100, 5.0))
	assert.NoError(t, c.IncrScore(ctx, "2026-04-21", "hot", "", 100, 3.0))

	got, err := c.Top(ctx, "2026-04-21", "hot", "", 10)
	assert.NoError(t, err)
	assert.Len(t, got, 1)
	assert.Equal(t, int64(100), got[0].ArticleId)
	assert.InDelta(t, 8.0, got[0].Score, 0.01)
}

func TestRedisRankingCache_Details(t *testing.T) {
	_, c := newRankingCache(t)
	ctx := context.Background()

	details := map[int64]domain.ArticleRanking{
		100: {ArticleId: 100, Title: "Go 泛型实战"},
		200: {ArticleId: 200, Title: "K8s Operator"},
	}
	assert.NoError(t, c.SetDetails(ctx, "2026-04-21", details))

	got, err := c.GetDetails(ctx, "2026-04-21", []int64{100, 200, 999})
	assert.NoError(t, err)
	assert.Len(t, got, 2)
	assert.Equal(t, "Go 泛型实战", got[100].Title)
	assert.Equal(t, "K8s Operator", got[200].Title)
	_, missing := got[999]
	assert.False(t, missing)
}

func TestRedisRankingCache_PrevRanks(t *testing.T) {
	_, c := newRankingCache(t)
	ctx := context.Background()

	ranks := map[int64]int{100: 1, 200: 2, 300: 5}
	assert.NoError(t, c.SnapshotRanks(ctx, "2026-04-21", "hot", "", ranks))

	got, err := c.GetPrevRanks(ctx, "2026-04-21", "hot", "", []int64{100, 200, 300, 999})
	assert.NoError(t, err)
	assert.Equal(t, 1, got[100])
	assert.Equal(t, 5, got[300])
	_, missing := got[999]
	assert.False(t, missing)
}

func TestRedisRankingCache_PrevRanksCategoryIsolation(t *testing.T) {
	_, c := newRankingCache(t)
	ctx := context.Background()

	assert.NoError(t, c.SnapshotRanks(ctx, "2026-04-21", "category", "tech", map[int64]int{100: 1, 200: 2}))
	assert.NoError(t, c.SnapshotRanks(ctx, "2026-04-21", "category", "career", map[int64]int{300: 1, 400: 2}))

	techMap, err := c.GetPrevRanks(ctx, "2026-04-21", "category", "tech", []int64{100, 200, 300})
	assert.NoError(t, err)
	assert.Equal(t, 1, techMap[100])
	assert.Equal(t, 2, techMap[200])
	_, hasCareer := techMap[300]
	assert.False(t, hasCareer, "tech 不应看到 career 的快照")

	careerMap, err := c.GetPrevRanks(ctx, "2026-04-21", "category", "career", []int64{300, 400, 100})
	assert.NoError(t, err)
	assert.Equal(t, 1, careerMap[300])
	_, hasTech := careerMap[100]
	assert.False(t, hasTech, "career 不应看到 tech 的快照")
}

func TestRedisRankingCache_DelDay(t *testing.T) {
	_, c := newRankingCache(t)
	ctx := context.Background()

	assert.NoError(t, c.ReplaceTop(ctx, "2026-04-21", "hot", "", []domain.ArticleRanking{{ArticleId: 1, Score: 1}}))
	assert.NoError(t, c.DelDay(ctx, "2026-04-21"))

	got, err := c.Top(ctx, "2026-04-21", "hot", "", 10)
	assert.NoError(t, err)
	assert.Len(t, got, 0)
}
