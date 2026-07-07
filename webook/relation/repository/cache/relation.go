package cache

import (
	"context"
	"fmt"
	"math/rand"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/webook/relation/consts"
	"github.com/webook/relation/domain"
)

type RelationCache interface {
	// GetStats 未命中返回 redis.Nil。
	GetStats(ctx context.Context, uid int64) (domain.RelationStats, error)
	SetStats(ctx context.Context, st domain.RelationStats) error
	// DelStats 写路径失效：一次删多个用户的计数缓存。
	DelStats(ctx context.Context, uids ...int64) error
}

type RedisRelationCache struct {
	cmd redis.Cmdable
}

func NewRedisRelationCache(cmd redis.Cmdable) RelationCache {
	return &RedisRelationCache{cmd: cmd}
}

func (c *RedisRelationCache) key(uid int64) string {
	return fmt.Sprintf(consts.RelationStatsPattern, uid)
}

func (c *RedisRelationCache) GetStats(ctx context.Context, uid int64) (domain.RelationStats, error) {
	data, err := c.cmd.HGetAll(ctx, c.key(uid)).Result()
	if err != nil {
		return domain.RelationStats{}, err
	}
	if len(data) == 0 {
		return domain.RelationStats{}, redis.Nil
	}
	followeeCnt, _ := strconv.ParseInt(data["followee_cnt"], 10, 64)
	followerCnt, _ := strconv.ParseInt(data["follower_cnt"], 10, 64)
	return domain.RelationStats{Uid: uid, FolloweeCnt: followeeCnt, FollowerCnt: followerCnt}, nil
}

func (c *RedisRelationCache) SetStats(ctx context.Context, st domain.RelationStats) error {
	key := c.key(st.Uid)
	jitter := time.Duration(rand.Int63n(int64(5 * time.Minute)))
	pipe := c.cmd.Pipeline()
	pipe.HSet(ctx, key, "followee_cnt", st.FolloweeCnt, "follower_cnt", st.FollowerCnt)
	pipe.Expire(ctx, key, consts.RelationStatsTTL+jitter)
	_, err := pipe.Exec(ctx)
	return err
}

func (c *RedisRelationCache) DelStats(ctx context.Context, uids ...int64) error {
	if len(uids) == 0 {
		return nil
	}
	keys := make([]string, 0, len(uids))
	for _, uid := range uids {
		keys = append(keys, c.key(uid))
	}
	return c.cmd.Del(ctx, keys...).Err()
}
