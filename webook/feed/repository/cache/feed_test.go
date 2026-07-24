package cache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/boyxs/train-go/webook/feed/domain"
)

// newTestCache 起一个 miniredis 支撑的 RedisFeedCache；cap 可按用例调小以验证裁剪。
func newTestCache(t *testing.T, cfg Config) (*RedisFeedCache, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	cmd := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return &RedisFeedCache{cmd: cmd, cfg: cfg}, mr
}

func defaultCfg() Config {
	return Config{InboxCap: 2000, InboxTTL: 168 * time.Hour, OutboxSize: 100, OutboxTTL: time.Hour}
}

func item(articleId, publishedAt int64) domain.FeedItem {
	return domain.FeedItem{ArticleId: articleId, PublishedAt: publishedAt}
}

func TestRedisFeedCache_InboxBuilt(t *testing.T) {
	c, _ := newTestCache(t, defaultCfg())
	ctx := context.Background()

	built, err := c.InboxBuilt(ctx, 1)
	require.NoError(t, err)
	assert.False(t, built, "未重建时应为 false")

	require.NoError(t, c.SaveInbox(ctx, 1, []domain.FeedItem{item(100, 1000)}, nil))
	built, err = c.InboxBuilt(ctx, 1)
	require.NoError(t, err)
	assert.True(t, built, "SaveInbox 后应为 true")
}

func TestRedisFeedCache_AppendInbox_and_ReadInbox(t *testing.T) {
	c, _ := newTestCache(t, defaultCfg())
	ctx := context.Background()

	// 扩散给 uid 1、2 三篇（时间递增）
	require.NoError(t, c.AppendInbox(ctx, []int64{1, 2}, item(100, 1000)))
	require.NoError(t, c.AppendInbox(ctx, []int64{1, 2}, item(200, 2000)))
	require.NoError(t, c.AppendInbox(ctx, []int64{1}, item(300, 3000)))

	// uid 1：首页按 score DESC
	got, err := c.ReadInbox(ctx, 1, 0, 10)
	require.NoError(t, err)
	assert.Equal(t, []domain.FeedItem{item(300, 3000), item(200, 2000), item(100, 1000)}, got)

	// uid 2 只有两篇
	got2, err := c.ReadInbox(ctx, 2, 0, 10)
	require.NoError(t, err)
	assert.Equal(t, []domain.FeedItem{item(200, 2000), item(100, 1000)}, got2)
}

func TestRedisFeedCache_ReadInbox_cursorPaging(t *testing.T) {
	c, _ := newTestCache(t, defaultCfg())
	ctx := context.Background()
	for _, it := range []domain.FeedItem{item(100, 1000), item(200, 2000), item(300, 3000)} {
		require.NoError(t, c.AppendInbox(ctx, []int64{1}, it))
	}

	// cursor=2000（开区间）→ 只返回 score<2000 的
	got, err := c.ReadInbox(ctx, 1, 2000, 10)
	require.NoError(t, err)
	assert.Equal(t, []domain.FeedItem{item(100, 1000)}, got)

	// limit=1 → 只取最新一条
	got, err = c.ReadInbox(ctx, 1, 0, 1)
	require.NoError(t, err)
	assert.Equal(t, []domain.FeedItem{item(300, 3000)}, got)
}

func TestRedisFeedCache_ReadInbox_empty(t *testing.T) {
	c, _ := newTestCache(t, defaultCfg())
	got, err := c.ReadInbox(context.Background(), 999, 0, 10)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestRedisFeedCache_AppendInbox_capTrim(t *testing.T) {
	cfg := defaultCfg()
	cfg.InboxCap = 3
	c, _ := newTestCache(t, cfg)
	ctx := context.Background()
	// 追加 5 篇，只应保留 score 最高的 3 篇
	for i := int64(1); i <= 5; i++ {
		require.NoError(t, c.AppendInbox(ctx, []int64{1}, item(i*100, i*1000)))
	}
	got, err := c.ReadInbox(ctx, 1, 0, 100)
	require.NoError(t, err)
	assert.Equal(t, []domain.FeedItem{item(500, 5000), item(400, 4000), item(300, 3000)}, got)
}

func TestRedisFeedCache_AppendInbox_idempotentAndTTL(t *testing.T) {
	c, mr := newTestCache(t, defaultCfg())
	ctx := context.Background()
	require.NoError(t, c.AppendInbox(ctx, []int64{1}, item(100, 1000)))
	require.NoError(t, c.AppendInbox(ctx, []int64{1}, item(100, 1000))) // 重投同 item

	got, err := c.ReadInbox(ctx, 1, 0, 100)
	require.NoError(t, err)
	assert.Equal(t, []domain.FeedItem{item(100, 1000)}, got, "重投应幂等不重复")

	ttl := mr.TTL("feed:inbox:1")
	assert.Greater(t, ttl, time.Duration(0), "应设置 TTL")
	assert.LessOrEqual(t, ttl, 168*time.Hour+5*time.Minute, "TTL 应在 7d+jitter 内")
}

func TestRedisFeedCache_SaveInbox(t *testing.T) {
	c, _ := newTestCache(t, defaultCfg())
	ctx := context.Background()
	// 先塞脏数据，验证 SaveInbox 会 DEL 旧
	require.NoError(t, c.AppendInbox(ctx, []int64{1}, item(999, 9999)))

	items := []domain.FeedItem{item(100, 1000), item(200, 2000)}
	require.NoError(t, c.SaveInbox(ctx, 1, items, []int64{7, 8}))

	got, err := c.ReadInbox(ctx, 1, 0, 100)
	require.NoError(t, err)
	assert.Equal(t, []domain.FeedItem{item(200, 2000), item(100, 1000)}, got, "应只含新 items（旧被 DEL）")

	bigvs, err := c.ReadBigv(ctx, 1)
	require.NoError(t, err)
	assert.ElementsMatch(t, []int64{7, 8}, bigvs)

	built, err := c.InboxBuilt(ctx, 1)
	require.NoError(t, err)
	assert.True(t, built)
}

func TestRedisFeedCache_SaveInbox_emptyStillBuilt(t *testing.T) {
	c, _ := newTestCache(t, defaultCfg())
	ctx := context.Background()
	require.NoError(t, c.SaveInbox(ctx, 1, nil, nil))

	built, err := c.InboxBuilt(ctx, 1)
	require.NoError(t, err)
	assert.True(t, built, "空关注用户也要置 built，避免每次读都重建")

	got, err := c.ReadInbox(ctx, 1, 0, 10)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestRedisFeedCache_SaveInbox_capTrim(t *testing.T) {
	cfg := defaultCfg()
	cfg.InboxCap = 2
	c, _ := newTestCache(t, cfg)
	ctx := context.Background()
	items := []domain.FeedItem{item(100, 1000), item(200, 2000), item(300, 3000)}
	require.NoError(t, c.SaveInbox(ctx, 1, items, nil))

	got, err := c.ReadInbox(ctx, 1, 0, 100)
	require.NoError(t, err)
	assert.Equal(t, []domain.FeedItem{item(300, 3000), item(200, 2000)}, got)
}

func TestRedisFeedCache_ReadBigv_empty(t *testing.T) {
	c, _ := newTestCache(t, defaultCfg())
	got, err := c.ReadBigv(context.Background(), 999)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestRedisFeedCache_Invalidate(t *testing.T) {
	c, _ := newTestCache(t, defaultCfg())
	ctx := context.Background()
	require.NoError(t, c.SaveInbox(ctx, 1, []domain.FeedItem{item(100, 1000)}, []int64{7}))
	require.NoError(t, c.SaveInbox(ctx, 2, []domain.FeedItem{item(200, 2000)}, []int64{8}))

	require.NoError(t, c.Invalidate(ctx, []int64{1, 2}))

	for _, uid := range []int64{1, 2} {
		built, err := c.InboxBuilt(ctx, uid)
		require.NoError(t, err)
		assert.False(t, built, "失效后 built 应为 false")

		got, err := c.ReadInbox(ctx, uid, 0, 10)
		require.NoError(t, err)
		assert.Empty(t, got, "失效后收件箱应为空")

		bigvs, err := c.ReadBigv(ctx, uid)
		require.NoError(t, err)
		assert.Empty(t, bigvs)
	}
}

func TestRedisFeedCache_InboxSince(t *testing.T) {
	c, _ := newTestCache(t, defaultCfg())
	ctx := context.Background()
	for _, it := range []domain.FeedItem{item(100, 1000), item(200, 2000), item(300, 3000)} {
		require.NoError(t, c.AppendInbox(ctx, []int64{1}, it))
	}

	ids, err := c.InboxSince(ctx, 1, 1500, 10) // score>1500 → 300,200（DESC）
	require.NoError(t, err)
	assert.Equal(t, []int64{300, 200}, ids)

	ids, err = c.InboxSince(ctx, 1, 0, 10) // 全部，DESC
	require.NoError(t, err)
	assert.Equal(t, []int64{300, 200, 100}, ids)

	ids, err = c.InboxSince(ctx, 1, 1500, 1) // limit 生效，只取最新一条
	require.NoError(t, err)
	assert.Equal(t, []int64{300}, ids)

	ids, err = c.InboxSince(ctx, 999, 0, 10) // 空收件箱
	require.NoError(t, err)
	assert.Empty(t, ids)
}

func TestRedisFeedCache_FillOutbox_and_ReadOutbox(t *testing.T) {
	c, _ := newTestCache(t, defaultCfg())
	ctx := context.Background()
	items := []domain.FeedItem{item(100, 1000), item(200, 2000), item(300, 3000)}
	require.NoError(t, c.FillOutbox(ctx, 7, items))

	// score DESC 首页
	got, err := c.ReadOutbox(ctx, 7, 0, 10)
	require.NoError(t, err)
	assert.Equal(t, []domain.FeedItem{item(300, 3000), item(200, 2000), item(100, 1000)}, got)

	// cursor 开区间翻页
	got, err = c.ReadOutbox(ctx, 7, 3000, 10)
	require.NoError(t, err)
	assert.Equal(t, []domain.FeedItem{item(200, 2000), item(100, 1000)}, got)

	// limit
	got, err = c.ReadOutbox(ctx, 7, 0, 1)
	require.NoError(t, err)
	assert.Equal(t, []domain.FeedItem{item(300, 3000)}, got)
}

func TestRedisFeedCache_ReadOutbox_empty(t *testing.T) {
	c, _ := newTestCache(t, defaultCfg())
	got, err := c.ReadOutbox(context.Background(), 999, 0, 10)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestRedisFeedCache_FillOutbox_capTrim(t *testing.T) {
	cfg := defaultCfg()
	cfg.OutboxSize = 2
	c, _ := newTestCache(t, cfg)
	ctx := context.Background()
	require.NoError(t, c.FillOutbox(ctx, 7, []domain.FeedItem{item(100, 1000), item(200, 2000), item(300, 3000)}))
	got, err := c.ReadOutbox(ctx, 7, 0, 100)
	require.NoError(t, err)
	assert.Equal(t, []domain.FeedItem{item(300, 3000), item(200, 2000)}, got)
}

func TestRedisFeedCache_AppendOutboxIfExists_notExist_noop(t *testing.T) {
	c, _ := newTestCache(t, defaultCfg())
	ctx := context.Background()
	// outbox 不存在 → 追加应被 Lua EXISTS 守卫挡下，不创建假全量
	require.NoError(t, c.AppendOutboxIfExists(ctx, 7, item(100, 1000)))
	got, err := c.ReadOutbox(ctx, 7, 0, 10)
	require.NoError(t, err)
	assert.Empty(t, got, "outbox 不存在时追加应为 no-op")
}

func TestRedisFeedCache_AppendOutboxIfExists_exist_appends(t *testing.T) {
	c, mr := newTestCache(t, defaultCfg())
	ctx := context.Background()
	require.NoError(t, c.FillOutbox(ctx, 7, []domain.FeedItem{item(100, 1000)}))
	require.NoError(t, c.AppendOutboxIfExists(ctx, 7, item(200, 2000)))

	got, err := c.ReadOutbox(ctx, 7, 0, 10)
	require.NoError(t, err)
	assert.Equal(t, []domain.FeedItem{item(200, 2000), item(100, 1000)}, got)

	ttl := mr.TTL("feed:outbox:7")
	assert.Greater(t, ttl, time.Duration(0), "追加应续 TTL")
}

func TestRedisFeedCache_AppendOutboxIfExists_capTrim(t *testing.T) {
	cfg := defaultCfg()
	cfg.OutboxSize = 2
	c, _ := newTestCache(t, cfg)
	ctx := context.Background()
	require.NoError(t, c.FillOutbox(ctx, 7, []domain.FeedItem{item(100, 1000)}))
	require.NoError(t, c.AppendOutboxIfExists(ctx, 7, item(200, 2000)))
	require.NoError(t, c.AppendOutboxIfExists(ctx, 7, item(300, 3000)))
	got, err := c.ReadOutbox(ctx, 7, 0, 100)
	require.NoError(t, err)
	assert.Equal(t, []domain.FeedItem{item(300, 3000), item(200, 2000)}, got)
}

func TestRedisFeedCache_DelOutbox(t *testing.T) {
	c, _ := newTestCache(t, defaultCfg())
	ctx := context.Background()
	require.NoError(t, c.FillOutbox(ctx, 7, []domain.FeedItem{item(100, 1000)}))
	require.NoError(t, c.DelOutbox(ctx, 7))
	got, err := c.ReadOutbox(ctx, 7, 0, 10)
	require.NoError(t, err)
	assert.Empty(t, got)
}
