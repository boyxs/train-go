package cache

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newThrottleCache(t *testing.T) (*miniredis.Miniredis, ThrottleCache) {
	t.Helper()
	mr := miniredis.RunT(t)
	cli := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return mr, NewRedisThrottleCache(cli)
}

func TestRedisThrottleCache(t *testing.T) {
	t.Run("Get 未配置 → (zero, false, nil)", func(t *testing.T) {
		_, c := newThrottleCache(t)
		cfg, ok, err := c.Get(context.Background(), 1)
		assert.NoError(t, err)
		assert.False(t, ok)
		assert.Equal(t, ThrottleConfig{}, cfg)
	})

	t.Run("Set + Get 来回", func(t *testing.T) {
		_, c := newThrottleCache(t)
		ctx := context.Background()
		want := ThrottleConfig{QPS: 5000, ShardWorkers: 8}
		require.NoError(t, c.Set(ctx, 1, want))
		got, ok, err := c.Get(ctx, 1)
		require.NoError(t, err)
		assert.True(t, ok)
		assert.Equal(t, want, got)
	})

	t.Run("key 带 migrator:throttle 前缀", func(t *testing.T) {
		mr, c := newThrottleCache(t)
		require.NoError(t, c.Set(context.Background(), 7, ThrottleConfig{QPS: 100}))
		assert.True(t, mr.Exists("migrator:throttle:7"))
		assert.False(t, mr.Exists("7"))
	})

	t.Run("Clear → Get 返 not found", func(t *testing.T) {
		_, c := newThrottleCache(t)
		ctx := context.Background()
		require.NoError(t, c.Set(ctx, 1, ThrottleConfig{QPS: 5000}))
		require.NoError(t, c.Clear(ctx, 1))
		_, ok, _ := c.Get(ctx, 1)
		assert.False(t, ok)
	})

	t.Run("损坏 JSON → Get 返 error", func(t *testing.T) {
		mr, c := newThrottleCache(t)
		require.NoError(t, mr.Set("migrator:throttle:1", "not-json"))
		_, _, err := c.Get(context.Background(), 1)
		assert.Error(t, err)
	})

	t.Run("Redis 真故障 → Get 返 error", func(t *testing.T) {
		mr, c := newThrottleCache(t)
		mr.Close()
		_, _, err := c.Get(context.Background(), 1)
		assert.Error(t, err)
	})
}

func TestThrottleConfig_Empty(t *testing.T) {
	assert.True(t, ThrottleConfig{}.Empty())
	assert.True(t, ThrottleConfig{QPS: 0, ShardWorkers: 0}.Empty())
	assert.False(t, ThrottleConfig{QPS: 1}.Empty())
	assert.False(t, ThrottleConfig{ShardWorkers: 1}.Empty())
}
