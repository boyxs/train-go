package source

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/webook/migrator/consts"
	"github.com/webook/migrator/domain"
	"github.com/webook/pkg/logger"
)

// ── 手写 mock BinlogClient ──────────────────────────────────
type stubBinlogClient struct {
	SubscribeFn func(ctx context.Context, fromPos string) (<-chan BinlogEvent, error)
	StopFn      func() error
}

func (s *stubBinlogClient) Subscribe(ctx context.Context, fromPos string) (<-chan BinlogEvent, error) {
	return s.SubscribeFn(ctx, fromPos)
}

func (s *stubBinlogClient) Stop() error {
	if s.StopFn != nil {
		return s.StopFn()
	}
	return nil
}

func TestCanalSource_IncrSubscribe(t *testing.T) {
	t.Run("3 个事件全部转换并推送", func(t *testing.T) {
		ch := make(chan BinlogEvent, 3)
		ch <- BinlogEvent{Op: "insert", Table: "article", PK: "1", BinlogPos: "mysql-bin.000001/4", EventTs: 1000}
		ch <- BinlogEvent{Op: "update", Table: "article", PK: "2", After: map[string]any{"title": "x"}, BinlogPos: "mysql-bin.000001/100", EventTs: 1500}
		ch <- BinlogEvent{Op: "delete", Table: "article", PK: "3", Before: map[string]any{"title": "y"}, BinlogPos: "mysql-bin.000001/200", EventTs: 2000}
		close(ch)

		client := &stubBinlogClient{
			SubscribeFn: func(_ context.Context, _ string) (<-chan BinlogEvent, error) {
				return ch, nil
			},
		}
		src := NewCanalSource(client, logger.NewNopLogger())

		out := make(chan ChangeEvent, 3)
		ctx, cancel := context.WithCancel(context.Background())
		// 异步消费 + ctx cancel 让 IncrSubscribe 退出
		go func() {
			collected := 0
			for range out {
				collected++
				if collected == 3 {
					cancel()
				}
			}
		}()
		err := src.IncrSubscribe(ctx, domain.Checkpoint{TaskId: 1}, out)
		assert.True(t, errors.Is(err, context.Canceled) || err == nil)
	})

	t.Run("ctx cancel 提前退出", func(t *testing.T) {
		ch := make(chan BinlogEvent) // 永不发送
		client := &stubBinlogClient{
			SubscribeFn: func(_ context.Context, _ string) (<-chan BinlogEvent, error) {
				return ch, nil
			},
		}
		src := NewCanalSource(client, logger.NewNopLogger())
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		out := make(chan ChangeEvent, 1)
		err := src.IncrSubscribe(ctx, domain.Checkpoint{}, out)
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("Subscribe 错误向上传播", func(t *testing.T) {
		boom := errors.New("canal connect failed")
		client := &stubBinlogClient{
			SubscribeFn: func(_ context.Context, _ string) (<-chan BinlogEvent, error) {
				return nil, boom
			},
		}
		src := NewCanalSource(client, logger.NewNopLogger())
		err := src.IncrSubscribe(context.Background(), domain.Checkpoint{}, nil)
		assert.ErrorIs(t, err, boom)
	})

	t.Run("CursorKind=binlog_pos 续订位点正确传递给 client", func(t *testing.T) {
		var capturedPos string
		ch := make(chan BinlogEvent)
		close(ch)
		client := &stubBinlogClient{
			SubscribeFn: func(_ context.Context, pos string) (<-chan BinlogEvent, error) {
				capturedPos = pos
				return ch, nil
			},
		}
		src := NewCanalSource(client, logger.NewNopLogger())
		ckpt := domain.Checkpoint{
			CursorKind:  consts.CursorKindBinlog,
			CursorValue: "mysql-bin.000005/100",
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_ = src.IncrSubscribe(ctx, ckpt, make(chan ChangeEvent, 1))
		assert.Equal(t, "mysql-bin.000005/100", capturedPos)
	})

	t.Run("CursorKind=gtid 当前未实现 → 显式拒绝", func(t *testing.T) {
		client := &stubBinlogClient{
			SubscribeFn: func(_ context.Context, _ string) (<-chan BinlogEvent, error) {
				t.Fatal("Subscribe should not be called for unsupported GTID cursor")
				return nil, nil
			},
		}
		src := NewCanalSource(client, logger.NewNopLogger())
		ckpt := domain.Checkpoint{
			CursorKind:  consts.CursorKindGTID,
			CursorValue: "uuid:1-100",
		}
		err := src.IncrSubscribe(context.Background(), ckpt, make(chan ChangeEvent, 1))
		assert.ErrorContains(t, err, "GTID cursor kind not yet implemented")
	})

	t.Run("不支持的 CursorKind → error，不调 Subscribe", func(t *testing.T) {
		client := &stubBinlogClient{
			SubscribeFn: func(_ context.Context, _ string) (<-chan BinlogEvent, error) {
				t.Fatal("Subscribe should not be called for unsupported cursor kind")
				return nil, nil
			},
		}
		src := NewCanalSource(client, logger.NewNopLogger())
		err := src.IncrSubscribe(context.Background(), domain.Checkpoint{CursorKind: "foobar"}, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported cursor kind")
	})
}

func TestCanalSource_Close(t *testing.T) {
	t.Run("Close 调用 client.Stop", func(t *testing.T) {
		var stopped bool
		src := NewCanalSource(&stubBinlogClient{StopFn: func() error {
			stopped = true
			return nil
		}}, logger.NewNopLogger())
		assert.NoError(t, src.Close())
		assert.True(t, stopped)
	})
}

func TestCanalSource_Lag(t *testing.T) {
	t.Run("无事件 → -1", func(t *testing.T) {
		src := NewCanalSource(&stubBinlogClient{}, logger.NewNopLogger())
		// type assert 到具体类型才能调 Lag（接口方法 LagReporter）
		r := src.(LagReporter)
		assert.Equal(t, int64(-1), r.Lag(99))
	})

	t.Run("有事件 → 大致等于 now - eventTs", func(t *testing.T) {
		ch := make(chan BinlogEvent, 1)
		now := time.Now().UnixMilli()
		ch <- BinlogEvent{Op: "insert", PK: "1", EventTs: now - 5000} // 5s ago
		close(ch)

		client := &stubBinlogClient{
			SubscribeFn: func(_ context.Context, _ string) (<-chan BinlogEvent, error) {
				return ch, nil
			},
		}
		src := NewCanalSource(client, logger.NewNopLogger())
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		out := make(chan ChangeEvent, 1)
		// 消费 1 个事件
		done := make(chan struct{})
		go func() {
			<-out
			close(done)
		}()
		_ = src.IncrSubscribe(ctx, domain.Checkpoint{TaskId: 7}, out)
		<-done

		lag := src.(LagReporter).Lag(7)
		assert.Greater(t, lag, int64(4000))
		assert.Less(t, lag, int64(10000))
	})
}

func TestParseBinlogPos(t *testing.T) {
	t.Run("parse 合法 file/pos", func(t *testing.T) {
		file, pos, ok := ParseBinlogPos("mysql-bin.000001/12345")
		require.True(t, ok)
		assert.Equal(t, "mysql-bin.000001", file)
		assert.Equal(t, int64(12345), pos)
	})
	t.Run("无 / → false", func(t *testing.T) {
		_, _, ok := ParseBinlogPos("invalid")
		assert.False(t, ok)
	})
	t.Run("/ 后非数字 → false", func(t *testing.T) {
		_, _, ok := ParseBinlogPos("file/abc")
		assert.False(t, ok)
	})
}
