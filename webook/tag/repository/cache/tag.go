package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/boyxs/train-go/webook/tag/consts"
	"github.com/boyxs/train-go/webook/tag/domain"
)

// TagCache 标签详情缓存（Cache-Aside）。只缓存 Detail 热点读；
// isFollowing（per-viewer 点查）/ typeahead / list 不缓存。
type TagCache interface {
	// GetDetail 未命中返回 redis.Nil。
	GetDetail(ctx context.Context, slug string) (domain.Tag, error)
	SetDetail(ctx context.Context, t domain.Tag) error
	// DelDetail 写路径失效：一次删多个 slug 的详情缓存。
	DelDetail(ctx context.Context, slugs ...string) error
}

type RedisTagCache struct {
	cmd redis.Cmdable
}

func NewRedisTagCache(cmd redis.Cmdable) TagCache {
	return &RedisTagCache{cmd: cmd}
}

func (c *RedisTagCache) key(slug string) string {
	return fmt.Sprintf(consts.TagDetailPattern, slug)
}

func (c *RedisTagCache) GetDetail(ctx context.Context, slug string) (domain.Tag, error) {
	data, err := c.cmd.Get(ctx, c.key(slug)).Bytes()
	if err != nil {
		return domain.Tag{}, err // redis.Nil = miss，交由调用方回源 DB
	}
	var t domain.Tag
	if err := json.Unmarshal(data, &t); err != nil {
		return domain.Tag{}, err
	}
	return t, nil
}

func (c *RedisTagCache) SetDetail(ctx context.Context, t domain.Tag) error {
	data, err := json.Marshal(t)
	if err != nil {
		return err
	}
	jitter := time.Duration(rand.Int63n(int64(5 * time.Minute)))
	return c.cmd.Set(ctx, c.key(t.Slug), data, consts.TagDetailTTL+jitter).Err()
}

func (c *RedisTagCache) DelDetail(ctx context.Context, slugs ...string) error {
	if len(slugs) == 0 {
		return nil
	}
	keys := make([]string, 0, len(slugs))
	for _, s := range slugs {
		keys = append(keys, c.key(s))
	}
	return c.cmd.Del(ctx, keys...).Err()
}
