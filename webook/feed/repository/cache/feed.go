package cache

import (
	"context"
	_ "embed"
	"fmt"
	"math/rand"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/boyxs/train-go/webook/feed/consts"
	"github.com/boyxs/train-go/webook/feed/domain"
)

//go:embed lua/outbox_append_if_exists.lua
var luaOutboxAppendIfExists string

// FeedCache 唯一碰 Redis 的层：收件箱(inbox)/发件箱(outbox)/大V集(bigv)/重建标记(built)。
// Lua、pipeline、jitter TTL 全部收拢在这里；上层 repository 只做协调。
type FeedCache interface {
	// ── inbox（收件箱）────────────────────────────────────
	// InboxBuilt 收件箱是否已完整重建（区分「未建」vs「建了但空」）。
	InboxBuilt(ctx context.Context, uid int64) (bool, error)
	// ReadInbox 读收件箱：cursor<=0 取首页，否则取 score<cursor（开区间）；score DESC，最多 limit 条。
	ReadInbox(ctx context.Context, uid, cursor int64, limit int) ([]domain.FeedItem, error)
	// AppendInbox 写扩散：把 item 追加进每个 uid 的收件箱（ZADD 幂等 + 裁 cap + 续 TTL）。
	AppendInbox(ctx context.Context, uids []int64, item domain.FeedItem) error
	// SaveInbox 重建：DEL 旧收件箱后 pipeline 写入 items + 大V集 + built 标记 + TTL（pipeline 是批量非
	// MULTI 事务；并发同 uid 重建靠结果幂等 + 后写覆盖，见 ARCHITECTURE §2.5「无锁双算」）。
	SaveInbox(ctx context.Context, uid int64, items []domain.FeedItem, bigvs []int64) error
	// ReadBigv 读该用户关注中的大 V uid 集合（读时归并其 outbox）。
	ReadBigv(ctx context.Context, uid int64) ([]int64, error)
	// Invalidate 失效重建：DEL 这些用户的 inbox+bigv+built，下次读全量重建。
	Invalidate(ctx context.Context, uids []int64) error
	// InboxSince 收件箱中 score > since 的候选文章 id（DESC，最多 limit 条；since<=0 取全部）。
	// 只返回原始成员，可见性（撤回/软删）由上层过滤——撤回文章仍留在收件箱、不能当新文章直接计数。
	InboxSince(ctx context.Context, uid, since int64, limit int) ([]int64, error)

	// ── outbox（发件箱）───────────────────────────────────
	ReadOutbox(ctx context.Context, authorId, cursor int64, limit int) ([]domain.FeedItem, error)
	FillOutbox(ctx context.Context, authorId int64, items []domain.FeedItem) error
	AppendOutboxIfExists(ctx context.Context, authorId int64, item domain.FeedItem) error
	DelOutbox(ctx context.Context, authorId int64) error
}

// Config feed 缓存调参（走 yaml feed 段，逐环境可调）。
type Config struct {
	InboxCap   int64         // 收件箱保留上限（ZREMRANGEBYRANK 裁 top N）
	InboxTTL   time.Duration // 收件箱 TTL（+0~5min jitter）
	OutboxSize int64         // 发件箱保留上限
	OutboxTTL  time.Duration // 发件箱 TTL（+0~5min jitter）
}

type RedisFeedCache struct {
	cmd redis.Cmdable
	cfg Config
}

func NewRedisFeedCache(cmd redis.Cmdable, cfg Config) FeedCache {
	return &RedisFeedCache{cmd: cmd, cfg: cfg}
}

func (c *RedisFeedCache) InboxBuilt(ctx context.Context, uid int64) (bool, error) {
	n, err := c.cmd.Exists(ctx, c.builtKey(uid)).Result()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (c *RedisFeedCache) ReadInbox(ctx context.Context, uid, cursor int64, limit int) ([]domain.FeedItem, error) {
	return c.readZSet(ctx, c.inboxKey(uid), cursor, limit)
}

func (c *RedisFeedCache) AppendInbox(ctx context.Context, uids []int64, item domain.FeedItem) error {
	if len(uids) == 0 {
		return nil
	}
	ttl := jitterTTL(c.cfg.InboxTTL)
	z := redis.Z{Score: float64(item.PublishedAt), Member: item.ArticleId}
	pipe := c.cmd.Pipeline()
	for _, uid := range uids {
		key := c.inboxKey(uid)
		pipe.ZAdd(ctx, key, z)                               // 幂等：同 member 覆盖
		pipe.ZRemRangeByRank(ctx, key, 0, -c.cfg.InboxCap-1) // 裁剪只留 score 最高的 cap 条
		pipe.Expire(ctx, key, ttl)
	}
	_, err := pipe.Exec(ctx)
	return err
}

func (c *RedisFeedCache) SaveInbox(ctx context.Context, uid int64, items []domain.FeedItem, bigvs []int64) error {
	inboxKey, bigvKey, builtKey := c.inboxKey(uid), c.bigvKey(uid), c.builtKey(uid)
	ttl := jitterTTL(c.cfg.InboxTTL)
	pipe := c.cmd.Pipeline()
	pipe.Del(ctx, inboxKey, bigvKey, builtKey) // 先清旧，重建幂等
	if len(items) > 0 {
		pipe.ZAdd(ctx, inboxKey, zsFromItems(items)...)
		pipe.ZRemRangeByRank(ctx, inboxKey, 0, -c.cfg.InboxCap-1)
		pipe.Expire(ctx, inboxKey, ttl)
	}
	if len(bigvs) > 0 {
		pipe.SAdd(ctx, bigvKey, int64sToAny(bigvs)...)
		pipe.Expire(ctx, bigvKey, ttl)
	}
	pipe.Set(ctx, builtKey, "1", ttl) // 空关注也置 built，避免每次读都重建
	_, err := pipe.Exec(ctx)
	return err
}

func (c *RedisFeedCache) ReadBigv(ctx context.Context, uid int64) ([]int64, error) {
	members, err := c.cmd.SMembers(ctx, c.bigvKey(uid)).Result()
	if err != nil {
		return nil, err
	}
	return parseInt64s(members), nil
}

func (c *RedisFeedCache) InboxSince(ctx context.Context, uid, since int64, limit int) ([]int64, error) {
	mint := "-inf"
	if since > 0 {
		mint = "(" + strconv.FormatInt(since, 10) // 开区间，排除等于 since 的条目
	}
	members, err := c.cmd.ZRevRangeByScore(ctx, c.inboxKey(uid), &redis.ZRangeBy{
		Min: mint, Max: "+inf", Count: int64(limit),
	}).Result()
	if err != nil {
		return nil, err
	}
	return parseInt64s(members), nil
}

func (c *RedisFeedCache) Invalidate(ctx context.Context, uids []int64) error {
	if len(uids) == 0 {
		return nil
	}
	keys := make([]string, 0, len(uids)*3)
	for _, uid := range uids {
		keys = append(keys, c.inboxKey(uid), c.bigvKey(uid), c.builtKey(uid))
	}
	return c.cmd.Del(ctx, keys...).Err()
}

func (c *RedisFeedCache) ReadOutbox(ctx context.Context, authorId, cursor int64, limit int) ([]domain.FeedItem, error) {
	return c.readZSet(ctx, c.outboxKey(authorId), cursor, limit)
}

// FillOutbox 回源填充：DEL 旧 → 写入源头的完整 top-N → 续 TTL。写的是全量，故不需 built 标记。
func (c *RedisFeedCache) FillOutbox(ctx context.Context, authorId int64, items []domain.FeedItem) error {
	key := c.outboxKey(authorId)
	ttl := jitterTTL(c.cfg.OutboxTTL)
	pipe := c.cmd.Pipeline()
	pipe.Del(ctx, key)
	if len(items) > 0 {
		pipe.ZAdd(ctx, key, zsFromItems(items)...)
		pipe.ZRemRangeByRank(ctx, key, 0, -c.cfg.OutboxSize-1)
		pipe.Expire(ctx, key, ttl)
	}
	_, err := pipe.Exec(ctx)
	return err
}

// AppendOutboxIfExists 存在才追加（Lua 原子）：outbox 冷（未回源）时 no-op，读路径会回源填全量。
func (c *RedisFeedCache) AppendOutboxIfExists(ctx context.Context, authorId int64, item domain.FeedItem) error {
	ttl := jitterTTL(c.cfg.OutboxTTL)
	return c.cmd.Eval(ctx, luaOutboxAppendIfExists,
		[]string{c.outboxKey(authorId)},
		item.PublishedAt, item.ArticleId, c.cfg.OutboxSize, ttl.Milliseconds(),
	).Err()
}

func (c *RedisFeedCache) DelOutbox(ctx context.Context, authorId int64) error {
	return c.cmd.Del(ctx, c.outboxKey(authorId)).Err()
}

// ── 内部工具 ──────────────────────────────────────────────

// readZSet 读 ZSET：cursor<=0 首页，否则 score<cursor（开区间）；score DESC，取 limit 条。
func (c *RedisFeedCache) readZSet(ctx context.Context, key string, cursor int64, limit int) ([]domain.FeedItem, error) {
	maxt := "+inf"
	if cursor > 0 {
		maxt = "(" + strconv.FormatInt(cursor, 10) // ( 前缀=开区间，排除等于 cursor 的条目
	}
	zs, err := c.cmd.ZRevRangeByScoreWithScores(ctx, key, &redis.ZRangeBy{
		Min:   "-inf",
		Max:   maxt,
		Count: int64(limit),
	}).Result()
	if err != nil {
		return nil, err
	}
	items := make([]domain.FeedItem, 0, len(zs))
	for _, z := range zs {
		member, _ := z.Member.(string)
		aid, err := strconv.ParseInt(member, 10, 64)
		if err != nil {
			continue
		}
		items = append(items, domain.FeedItem{ArticleId: aid, PublishedAt: int64(z.Score)})
	}
	return items, nil
}

func (c *RedisFeedCache) inboxKey(uid int64) string { return fmt.Sprintf(consts.InboxPattern, uid) }
func (c *RedisFeedCache) builtKey(uid int64) string {
	return fmt.Sprintf(consts.InboxBuiltPattern, uid)
}
func (c *RedisFeedCache) bigvKey(uid int64) string   { return fmt.Sprintf(consts.BigvPattern, uid) }
func (c *RedisFeedCache) outboxKey(aid int64) string { return fmt.Sprintf(consts.OutboxPattern, aid) }

func zsFromItems(items []domain.FeedItem) []redis.Z {
	zs := make([]redis.Z, 0, len(items))
	for _, it := range items {
		zs = append(zs, redis.Z{Score: float64(it.PublishedAt), Member: it.ArticleId})
	}
	return zs
}

func jitterTTL(base time.Duration) time.Duration {
	return base + time.Duration(rand.Int63n(int64(5*time.Minute)))
}

func int64sToAny(xs []int64) []interface{} {
	out := make([]interface{}, 0, len(xs))
	for _, x := range xs {
		out = append(out, x)
	}
	return out
}

func parseInt64s(ss []string) []int64 {
	out := make([]int64, 0, len(ss))
	for _, s := range ss {
		if v, err := strconv.ParseInt(s, 10, 64); err == nil {
			out = append(out, v)
		}
	}
	return out
}
