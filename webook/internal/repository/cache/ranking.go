package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/boyxs/train-go/webook/internal/consts"
	"github.com/boyxs/train-go/webook/internal/domain"
	"github.com/boyxs/train-go/webook/pkg/logger"
)

// jitteredRankingTTL 基础 TTL + 0~5min 随机抖动。
// 如果所有 key 同一时刻过期，Redis 内存瞬时大量释放 + 下游大量 miss 回源打爆 DB，这就是缓存雪崩。
// 加 ±5min 让过期时间打散。
func jitteredRankingTTL() time.Duration {
	return consts.ArticleRankingTTL + time.Duration(rand.Int63n(int64(5*time.Minute)))
}

type RankingCache interface {
	ReplaceTop(ctx context.Context, date, dim, cat string, items []domain.ArticleRanking) error
	Top(ctx context.Context, date, dim, cat string, limit int) ([]domain.ArticleRanking, error)
	IncrScore(ctx context.Context, date, dim, cat string, articleId int64, delta float64) error

	SetDetails(ctx context.Context, date string, details map[int64]domain.ArticleRanking) error
	GetDetails(ctx context.Context, date string, articleIds []int64) (map[int64]domain.ArticleRanking, error)

	SnapshotRanks(ctx context.Context, date, dim, cat string, ranks map[int64]int) error
	GetPrevRanks(ctx context.Context, date, dim, cat string, articleIds []int64) (map[int64]int, error)

	DelDay(ctx context.Context, date string) error
}

type RedisArticleRankingCache struct {
	cmd redis.Cmdable
	l   logger.LoggerX
}

func NewRedisArticleRankingCache(cmd redis.Cmdable, l logger.LoggerX) RankingCache {
	return &RedisArticleRankingCache{cmd: cmd, l: l}
}

func (c *RedisArticleRankingCache) zsetKey(date, dim, cat string) string {
	if cat != "" {
		return fmt.Sprintf(consts.ArticleRankingCategoryZSetPattern, date, cat)
	}
	return fmt.Sprintf(consts.ArticleRankingZSetPattern, date, dim)
}

func (c *RedisArticleRankingCache) detailKey(date string) string {
	return fmt.Sprintf(consts.ArticleRankingDetailPattern, date)
}

// prevRankKey 分区榜按 cat 隔离。没有这个隔离时，tech/career/life 5 个分区共用同一个
// prevRank Hash，每次 ReplaceTop 互相覆盖，趋势全乱。cat="" 是总榜。
func (c *RedisArticleRankingCache) prevRankKey(date, dim, cat string) string {
	return fmt.Sprintf(consts.ArticleRankingPrevRankPattern, date, dim, cat)
}

// ReplaceTop 三步原子操作（Pipeline 合并为一次网络往返）：Del 清旧榜 → ZAdd 塞新榜 → Expire 续命。
// 用 Pipeline 而非单独调用，避免中间态：若 Del 成功后 ZAdd 失败，ZSet 彻底变空用户看到空白榜。
func (c *RedisArticleRankingCache) ReplaceTop(ctx context.Context, date, dim, cat string, items []domain.ArticleRanking) error {
	key := c.zsetKey(date, dim, cat)
	pipe := c.cmd.Pipeline()
	pipe.Del(ctx, key)
	if len(items) > 0 {
		zs := make([]redis.Z, 0, len(items))
		for _, it := range items {
			zs = append(zs, redis.Z{Score: it.Score, Member: it.ArticleId})
		}
		pipe.ZAdd(ctx, key, zs...)
	}
	pipe.Expire(ctx, key, jitteredRankingTTL())
	_, err := pipe.Exec(ctx)
	return err
}

func (c *RedisArticleRankingCache) Top(ctx context.Context, date, dim, cat string, limit int) ([]domain.ArticleRanking, error) {
	key := c.zsetKey(date, dim, cat)
	zs, err := c.cmd.ZRevRangeWithScores(ctx, key, 0, int64(limit-1)).Result()
	if err != nil {
		return nil, err
	}
	items := make([]domain.ArticleRanking, 0, len(zs))
	for i, z := range zs {
		id, err := parseInt64(z.Member)
		if err != nil {
			continue
		}
		items = append(items, domain.ArticleRanking{Rank: i + 1, ArticleId: id, Score: z.Score})
	}
	return items, nil
}

func (c *RedisArticleRankingCache) IncrScore(ctx context.Context, date, dim, cat string, articleId int64, delta float64) error {
	key := c.zsetKey(date, dim, cat)
	pipe := c.cmd.Pipeline()
	pipe.ZIncrBy(ctx, key, delta, strconv.FormatInt(articleId, 10))
	pipe.Expire(ctx, key, jitteredRankingTTL())
	_, err := pipe.Exec(ctx)
	return err
}

func (c *RedisArticleRankingCache) SetDetails(ctx context.Context, date string, details map[int64]domain.ArticleRanking) error {
	if len(details) == 0 {
		return nil
	}
	key := c.detailKey(date)
	values := make(map[string]any, len(details))
	for id, item := range details {
		bs, err := json.Marshal(item)
		if err != nil {
			return err
		}
		values[strconv.FormatInt(id, 10)] = bs
	}
	pipe := c.cmd.Pipeline()
	pipe.HSet(ctx, key, values)
	pipe.Expire(ctx, key, jitteredRankingTTL())
	_, err := pipe.Exec(ctx)
	return err
}

func (c *RedisArticleRankingCache) GetDetails(ctx context.Context, date string, articleIds []int64) (map[int64]domain.ArticleRanking, error) {
	if len(articleIds) == 0 {
		return map[int64]domain.ArticleRanking{}, nil
	}
	key := c.detailKey(date)
	fields := make([]string, 0, len(articleIds))
	for _, id := range articleIds {
		fields = append(fields, strconv.FormatInt(id, 10))
	}
	vals, err := c.cmd.HMGet(ctx, key, fields...).Result()
	if err != nil {
		return nil, err
	}
	result := make(map[int64]domain.ArticleRanking, len(articleIds))
	for i, v := range vals {
		if v == nil {
			continue
		}
		s, ok := v.(string)
		if !ok {
			c.l.Warn(ctx, "GetDetails 类型断言失败", logger.Int64("articleId", articleIds[i]))
			continue
		}
		var item domain.ArticleRanking
		if err := json.Unmarshal([]byte(s), &item); err != nil {
			c.l.Warn(ctx, "GetDetails JSON 反序列化失败",
				logger.Int64("articleId", articleIds[i]), logger.Error(err))
			continue
		}
		result[articleIds[i]] = item
	}
	return result, nil
}

func (c *RedisArticleRankingCache) SnapshotRanks(ctx context.Context, date, dim, cat string, ranks map[int64]int) error {
	if len(ranks) == 0 {
		return nil
	}
	key := c.prevRankKey(date, dim, cat)
	values := make(map[string]any, len(ranks))
	for id, r := range ranks {
		values[strconv.FormatInt(id, 10)] = r
	}
	pipe := c.cmd.Pipeline()
	pipe.Del(ctx, key)
	pipe.HSet(ctx, key, values)
	pipe.Expire(ctx, key, jitteredRankingTTL())
	_, err := pipe.Exec(ctx)
	return err
}

func (c *RedisArticleRankingCache) GetPrevRanks(ctx context.Context, date, dim, cat string, articleIds []int64) (map[int64]int, error) {
	if len(articleIds) == 0 {
		return map[int64]int{}, nil
	}
	key := c.prevRankKey(date, dim, cat)
	fields := make([]string, 0, len(articleIds))
	for _, id := range articleIds {
		fields = append(fields, strconv.FormatInt(id, 10))
	}
	vals, err := c.cmd.HMGet(ctx, key, fields...).Result()
	if err != nil {
		return nil, err
	}
	result := make(map[int64]int, len(articleIds))
	for i, v := range vals {
		if v == nil {
			continue
		}
		s, ok := v.(string)
		if !ok {
			c.l.Warn(ctx, "GetPrevRanks 类型断言失败", logger.Int64("articleId", articleIds[i]))
			continue
		}
		r, err := strconv.Atoi(s)
		if err != nil {
			c.l.Warn(ctx, "GetPrevRanks 解析 rank 失败",
				logger.Int64("articleId", articleIds[i]), logger.Error(err))
			continue
		}
		result[articleIds[i]] = r
	}
	return result, nil
}

func (c *RedisArticleRankingCache) DelDay(ctx context.Context, date string) error {
	totalDims := []domain.Dimension{domain.DimensionHot, domain.DimensionNew, domain.DimensionBest}
	keys := []string{c.detailKey(date)}
	for _, d := range totalDims {
		keys = append(keys,
			fmt.Sprintf(consts.ArticleRankingZSetPattern, date, string(d)),
			fmt.Sprintf(consts.ArticleRankingPrevRankPattern, date, string(d), ""),
		)
	}
	for _, cat := range consts.AllCategories {
		keys = append(keys,
			fmt.Sprintf(consts.ArticleRankingCategoryZSetPattern, date, cat),
			fmt.Sprintf(consts.ArticleRankingPrevRankPattern, date, string(domain.DimensionCategory), cat),
		)
	}
	return c.cmd.Del(ctx, keys...).Err()
}

func parseInt64(v any) (int64, error) {
	switch x := v.(type) {
	case int64:
		return x, nil
	case string:
		return strconv.ParseInt(x, 10, 64)
	default:
		return 0, fmt.Errorf("unexpected member type %T", v)
	}
}
