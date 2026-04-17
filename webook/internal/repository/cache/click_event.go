package cache

import (
	"context"
	"encoding/json"
	"math/rand"
	"time"

	"github.com/webook/internal/consts"
	"github.com/webook/internal/domain"
	"github.com/redis/go-redis/v9"
)

type ClickEventCache interface {
	GetDashboard(ctx context.Context) (domain.ClickEventDashboard, error)
	SetDashboard(ctx context.Context, data domain.ClickEventDashboard) error
	DelDashboard(ctx context.Context) error
}

type RedisAIClickEventCache struct {
	cmd redis.Cmdable
}

func NewRedisAIClickEventCache(cmd redis.Cmdable) ClickEventCache {
	return &RedisAIClickEventCache{cmd: cmd}
}

func (c *RedisAIClickEventCache) GetDashboard(ctx context.Context) (domain.ClickEventDashboard, error) {
	val, err := c.cmd.Get(ctx, consts.ClickEventDashboardKey).Result()
	if err != nil {
		return domain.ClickEventDashboard{}, err
	}
	var data domain.ClickEventDashboard
	err = json.Unmarshal([]byte(val), &data)
	return data, err
}

func (c *RedisAIClickEventCache) SetDashboard(ctx context.Context, data domain.ClickEventDashboard) error {
	bs, err := json.Marshal(data)
	if err != nil {
		return err
	}
	// TTL + 随机 jitter 防雪崩
	jitter := time.Duration(rand.Int63n(int64(5 * time.Minute)))
	return c.cmd.Set(ctx, consts.ClickEventDashboardKey, bs, consts.ClickEventDashboardTTL+jitter).Err()
}

func (c *RedisAIClickEventCache) DelDashboard(ctx context.Context) error {
	return c.cmd.Del(ctx, consts.ClickEventDashboardKey).Err()
}
