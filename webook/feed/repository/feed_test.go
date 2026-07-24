package repository

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/boyxs/train-go/webook/feed/domain"
	"github.com/boyxs/train-go/webook/feed/repository/cache"
)

// newTestRepo：真实 RedisFeedCache（miniredis 支撑）→ 验证 repo↔cache 委托贯通。
func newTestRepo(t *testing.T) FeedRepository {
	t.Helper()
	mr := miniredis.RunT(t)
	cmd := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	c := cache.NewRedisFeedCache(cmd, cache.Config{
		InboxCap: 2000, InboxTTL: 168 * time.Hour, OutboxSize: 100, OutboxTTL: time.Hour,
	})
	return NewCacheFeedRepository(c)
}

func item(articleId, publishedAt int64) domain.FeedItem {
	return domain.FeedItem{ArticleId: articleId, PublishedAt: publishedAt}
}

func TestCacheFeedRepository_Inbox(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	require.NoError(t, repo.AppendInbox(ctx, []int64{1}, item(100, 1000)))
	require.NoError(t, repo.AppendInbox(ctx, []int64{1}, item(200, 2000)))

	got, err := repo.ReadInbox(ctx, 1, 0, 10)
	require.NoError(t, err)
	assert.Equal(t, []domain.FeedItem{item(200, 2000), item(100, 1000)}, got)
}

func TestCacheFeedRepository_SaveInbox_and_Built_and_Bigv(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	built, err := repo.InboxBuilt(ctx, 1)
	require.NoError(t, err)
	assert.False(t, built)

	require.NoError(t, repo.SaveInbox(ctx, 1, []domain.FeedItem{item(100, 1000)}, []int64{7, 8}))

	built, err = repo.InboxBuilt(ctx, 1)
	require.NoError(t, err)
	assert.True(t, built)

	bigvs, err := repo.ReadBigv(ctx, 1)
	require.NoError(t, err)
	assert.ElementsMatch(t, []int64{7, 8}, bigvs)
}

func TestCacheFeedRepository_Invalidate(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.SaveInbox(ctx, 1, []domain.FeedItem{item(100, 1000)}, []int64{7}))

	require.NoError(t, repo.Invalidate(ctx, []int64{1}))

	built, err := repo.InboxBuilt(ctx, 1)
	require.NoError(t, err)
	assert.False(t, built, "失效后 built 应为 false")
}

func TestCacheFeedRepository_Outbox(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	// 不存在时追加应 no-op（Lua EXISTS 守卫）
	require.NoError(t, repo.AppendOutboxIfExists(ctx, 7, item(100, 1000)))
	got, err := repo.ReadOutbox(ctx, 7, 0, 10)
	require.NoError(t, err)
	assert.Empty(t, got)

	// 回源填充后可读、可追加
	require.NoError(t, repo.FillOutbox(ctx, 7, []domain.FeedItem{item(100, 1000)}))
	require.NoError(t, repo.AppendOutboxIfExists(ctx, 7, item(200, 2000)))
	got, err = repo.ReadOutbox(ctx, 7, 0, 10)
	require.NoError(t, err)
	assert.Equal(t, []domain.FeedItem{item(200, 2000), item(100, 1000)}, got)

	// 删除后为空
	require.NoError(t, repo.DelOutbox(ctx, 7))
	got, err = repo.ReadOutbox(ctx, 7, 0, 10)
	require.NoError(t, err)
	assert.Empty(t, got)
}
