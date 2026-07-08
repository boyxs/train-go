package cache

import (
	"context"
	"errors"
	"math/rand"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/boyxs/train-go/webook/comment/consts"
)

// CommentCache 评论缓存。P0 只缓存评论总数（列表/最热缓存在 core 聚合层）。
type CommentCache interface {
	GetCount(ctx context.Context, biz string, bizId int64) (int64, error)
	SetCount(ctx context.Context, biz string, bizId, cnt int64) error
	DelCount(ctx context.Context, biz string, bizId int64) error
}

// IsMiss 判断是否缓存未命中。
func IsMiss(err error) bool {
	return errors.Is(err, redis.Nil)
}

type RedisCommentCache struct {
	cmd redis.Cmdable
}

func NewRedisCommentCache(cmd redis.Cmdable) CommentCache {
	return &RedisCommentCache{cmd: cmd}
}

func (c *RedisCommentCache) GetCount(ctx context.Context, biz string, bizId int64) (int64, error) {
	return c.cmd.Get(ctx, consts.CommentCountKey(biz, bizId)).Int64()
}

func (c *RedisCommentCache) SetCount(ctx context.Context, biz string, bizId, cnt int64) error {
	// TTL 10min + 0~5min 随机 jitter，防缓存雪崩
	ttl := 10*time.Minute + time.Duration(rand.Intn(300))*time.Second
	return c.cmd.Set(ctx, consts.CommentCountKey(biz, bizId), cnt, ttl).Err()
}

func (c *RedisCommentCache) DelCount(ctx context.Context, biz string, bizId int64) error {
	return c.cmd.Del(ctx, consts.CommentCountKey(biz, bizId)).Err()
}
