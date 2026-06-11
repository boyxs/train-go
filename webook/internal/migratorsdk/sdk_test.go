package migratorsdk

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/webook/pkg/logger"
)

// ── NoOp 实现 ───────────────────────────────────────────

func TestNoOpSwitchReader(t *testing.T) {
	r := NewNoOpSwitchReader()
	side, err := r.ChooseSide(context.Background(), "any", 12345)
	assert.NoError(t, err)
	assert.Equal(t, SideOld, side)
}

func TestNoOpDualWriter(t *testing.T) {
	w := NewNoOpDualWriter()
	t.Run("fn 仅被调用 1 次 + side=Old", func(t *testing.T) {
		var calls int32
		var capturedSide Side
		err := w.Write(context.Background(), "any", func(side Side) error {
			atomic.AddInt32(&calls, 1)
			capturedSide = side
			return nil
		})
		assert.NoError(t, err)
		assert.Equal(t, int32(1), atomic.LoadInt32(&calls))
		assert.Equal(t, SideOld, capturedSide)
	})

	t.Run("fn 错误透传", func(t *testing.T) {
		boom := errors.New("biz failed")
		err := w.Write(context.Background(), "any", func(_ Side) error {
			return boom
		})
		assert.ErrorIs(t, err, boom)
	})
}

// ── Redis 实现 ──────────────────────────────────────────

func newRedis(t *testing.T) (*miniredis.Miniredis, redis.Cmdable) {
	t.Helper()
	mr := miniredis.RunT(t)
	cli := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return mr, cli
}

func TestRedisSwitchReader(t *testing.T) {
	t.Run("stage 未设置（默认 SRC_ONLY） → SideOld", func(t *testing.T) {
		_, cli := newRedis(t)
		r := NewRedisSwitchReader(cli, logger.NewNopLogger())
		side, err := r.ChooseSide(context.Background(), "t1", 100)
		require.NoError(t, err)
		assert.Equal(t, SideOld, side)
	})

	t.Run("stage=SRC_FIRST + gray=0 → SideOld", func(t *testing.T) {
		mr, cli := newRedis(t)
		require.NoError(t, mr.Set(keyStage+"t1", stageSrcFirst))
		require.NoError(t, mr.Set(keyGray+"t1", "0"))
		r := NewRedisSwitchReader(cli, logger.NewNopLogger())
		side, _ := r.ChooseSide(context.Background(), "t1", 42)
		assert.Equal(t, SideOld, side)
	})

	t.Run("stage=SRC_FIRST + gray=100 → SideNew", func(t *testing.T) {
		mr, cli := newRedis(t)
		require.NoError(t, mr.Set(keyStage+"t1", stageSrcFirst))
		require.NoError(t, mr.Set(keyGray+"t1", "100"))
		r := NewRedisSwitchReader(cli, logger.NewNopLogger())
		side, _ := r.ChooseSide(context.Background(), "t1", 42)
		assert.Equal(t, SideNew, side)
	})

	t.Run("stage=SRC_FIRST + gray=50 → 同 hashKey 落同一桶（read-your-write 保障）", func(t *testing.T) {
		mr, cli := newRedis(t)
		require.NoError(t, mr.Set(keyStage+"t1", stageSrcFirst))
		require.NoError(t, mr.Set(keyGray+"t1", "50"))
		r := NewRedisSwitchReader(cli, logger.NewNopLogger())

		// 同 hashKey 多次调用必须返回同一 side
		side1, _ := r.ChooseSide(context.Background(), "t1", 999)
		side2, _ := r.ChooseSide(context.Background(), "t1", 999)
		side3, _ := r.ChooseSide(context.Background(), "t1", 999)
		assert.Equal(t, side1, side2)
		assert.Equal(t, side1, side3)
	})

	t.Run("stage=SRC_FIRST + gray=50 → 不同 hashKey 大致 50% 落 NEW", func(t *testing.T) {
		mr, cli := newRedis(t)
		require.NoError(t, mr.Set(keyStage+"t1", stageSrcFirst))
		require.NoError(t, mr.Set(keyGray+"t1", "50"))
		r := NewRedisSwitchReader(cli, logger.NewNopLogger())

		var newCount int
		const n = 1000
		for i := int64(0); i < n; i++ {
			side, _ := r.ChooseSide(context.Background(), "t1", i)
			if side == SideNew {
				newCount++
			}
		}
		// FNV hash 分布应在 50% ± 10% 内（n=1000 时不会偏太多）
		assert.InDelta(t, n/2, newCount, float64(n)*0.1)
	})

	t.Run("stage=DST_FIRST → SideNew（无视 gray）", func(t *testing.T) {
		mr, cli := newRedis(t)
		require.NoError(t, mr.Set(keyStage+"t1", stageDstFirst))
		require.NoError(t, mr.Set(keyGray+"t1", "0"))
		r := NewRedisSwitchReader(cli, logger.NewNopLogger())
		side, _ := r.ChooseSide(context.Background(), "t1", 42)
		assert.Equal(t, SideNew, side)
	})

	t.Run("stage=DST_ONLY → SideNew", func(t *testing.T) {
		mr, cli := newRedis(t)
		require.NoError(t, mr.Set(keyStage+"t1", stageDstOnly))
		r := NewRedisSwitchReader(cli, logger.NewNopLogger())
		side, _ := r.ChooseSide(context.Background(), "t1", 42)
		assert.Equal(t, SideNew, side)
	})

	t.Run("Redis 故障 → 降级 SideOld 不抛错", func(t *testing.T) {
		mr, cli := newRedis(t)
		mr.Close() // 模拟 Redis 不可达
		r := NewRedisSwitchReader(cli, logger.NewNopLogger())
		side, err := r.ChooseSide(context.Background(), "t1", 42)
		assert.NoError(t, err, "Redis 故障必须降级而非抛错给业务")
		assert.Equal(t, SideOld, side)
	})
}

// ── RedisDualWriter ─────────────────────────────────────

type captureRecorder struct {
	called int32
	last   error
}

func (c *captureRecorder) Record(_ context.Context, _ string, cause error) {
	atomic.AddInt32(&c.called, 1)
	c.last = cause
}

func TestRedisDualWriter(t *testing.T) {
	t.Run("stage=SRC_ONLY → 仅写 OLD", func(t *testing.T) {
		_, cli := newRedis(t)
		w := NewRedisDualWriter(cli, nil, logger.NewNopLogger())
		var sides []Side
		err := w.Write(context.Background(), "t1", func(side Side) error {
			sides = append(sides, side)
			return nil
		})
		require.NoError(t, err)
		assert.Equal(t, []Side{SideOld}, sides)
	})

	t.Run("stage=SRC_FIRST + OLD 成 NEW 成 → 写两侧不报错", func(t *testing.T) {
		mr, cli := newRedis(t)
		require.NoError(t, mr.Set(keyStage+"t1", stageSrcFirst))
		w := NewRedisDualWriter(cli, nil, logger.NewNopLogger())
		var sides []Side
		err := w.Write(context.Background(), "t1", func(side Side) error {
			sides = append(sides, side)
			return nil
		})
		require.NoError(t, err)
		assert.Equal(t, []Side{SideOld, SideNew}, sides)
	})

	t.Run("stage=SRC_FIRST + OLD 失败 → 业务报错，NEW 不调", func(t *testing.T) {
		mr, cli := newRedis(t)
		require.NoError(t, mr.Set(keyStage+"t1", stageSrcFirst))
		w := NewRedisDualWriter(cli, nil, logger.NewNopLogger())
		boom := errors.New("OLD failed")
		var newCalled bool
		err := w.Write(context.Background(), "t1", func(side Side) error {
			if side == SideOld {
				return boom
			}
			newCalled = true
			return nil
		})
		assert.ErrorIs(t, err, boom)
		assert.False(t, newCalled, "OLD 失败时 NEW 不应该被调")
	})

	t.Run("stage=SRC_FIRST + NEW 失败 → 业务不报错，Recorder 被调", func(t *testing.T) {
		mr, cli := newRedis(t)
		require.NoError(t, mr.Set(keyStage+"t1", stageSrcFirst))
		rec := &captureRecorder{}
		w := NewRedisDualWriter(cli, rec, logger.NewNopLogger())
		newErr := errors.New("NEW dao failed")
		err := w.Write(context.Background(), "t1", func(side Side) error {
			if side == SideNew {
				return newErr
			}
			return nil
		})
		assert.NoError(t, err, "SRC_FIRST 阶段 NEW 失败不能向业务抛错")
		assert.Equal(t, int32(1), atomic.LoadInt32(&rec.called))
		assert.ErrorIs(t, rec.last, newErr)
	})

	t.Run("stage=DST_FIRST + OLD 失败 → 业务报错", func(t *testing.T) {
		mr, cli := newRedis(t)
		require.NoError(t, mr.Set(keyStage+"t1", stageDstFirst))
		w := NewRedisDualWriter(cli, nil, logger.NewNopLogger())
		boom := errors.New("OLD dao failed")
		err := w.Write(context.Background(), "t1", func(side Side) error {
			if side == SideOld {
				return boom
			}
			return nil
		})
		assert.ErrorIs(t, err, boom)
	})

	t.Run("stage=DST_FIRST + NEW 失败 → 业务报错（严格双写）", func(t *testing.T) {
		mr, cli := newRedis(t)
		require.NoError(t, mr.Set(keyStage+"t1", stageDstFirst))
		w := NewRedisDualWriter(cli, nil, logger.NewNopLogger())
		boom := errors.New("NEW dao failed")
		err := w.Write(context.Background(), "t1", func(side Side) error {
			if side == SideNew {
				return boom
			}
			return nil
		})
		assert.ErrorIs(t, err, boom, "DST_FIRST 阶段 NEW 失败必须报错（保读写一致）")
	})

	t.Run("stage=DST_ONLY → 仅写 NEW", func(t *testing.T) {
		mr, cli := newRedis(t)
		require.NoError(t, mr.Set(keyStage+"t1", stageDstOnly))
		w := NewRedisDualWriter(cli, nil, logger.NewNopLogger())
		var sides []Side
		err := w.Write(context.Background(), "t1", func(side Side) error {
			sides = append(sides, side)
			return nil
		})
		require.NoError(t, err)
		assert.Equal(t, []Side{SideNew}, sides)
	})

	t.Run("Redis 故障 → 降级 SRC_ONLY（业务行为不变）", func(t *testing.T) {
		mr, cli := newRedis(t)
		mr.Close()
		w := NewRedisDualWriter(cli, nil, logger.NewNopLogger())
		var sides []Side
		err := w.Write(context.Background(), "t1", func(side Side) error {
			sides = append(sides, side)
			return nil
		})
		require.NoError(t, err, "Redis 故障必须降级而非阻塞业务")
		assert.Equal(t, []Side{SideOld}, sides, "降级后只写 OLD（业务原始行为）")
	})

	t.Run("NoOpFailureRecorder nil-safe", func(t *testing.T) {
		rec := NoOpFailureRecorder{L: nil}
		assert.NotPanics(t, func() {
			rec.Record(context.Background(), "x", errors.New("e"))
		})
	})
}

func TestHashMod100(t *testing.T) {
	t.Run("同一 hashKey 多次调用结果一致", func(t *testing.T) {
		v1 := hashMod100(12345)
		v2 := hashMod100(12345)
		assert.Equal(t, v1, v2)
	})
	t.Run("不同 hashKey 一般不同", func(t *testing.T) {
		distinct := map[uint32]struct{}{}
		for i := int64(0); i < 100; i++ {
			distinct[hashMod100(i)] = struct{}{}
		}
		// 100 个不同 hashKey 至少落到 30 个不同桶（FNV 分布足够散）
		assert.Greater(t, len(distinct), 30, "hash 分布太集中")
	})
	t.Run("结果范围 [0, 100)", func(t *testing.T) {
		for i := int64(0); i < 1000; i++ {
			v := hashMod100(i)
			assert.Less(t, v, uint32(100))
		}
	})
}
