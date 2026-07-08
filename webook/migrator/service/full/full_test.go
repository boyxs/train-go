package full

import (
	"context"
	"errors"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/boyxs/train-go/webook/migrator/domain"
	"github.com/boyxs/train-go/webook/migrator/pipeline/sink"
	"github.com/boyxs/train-go/webook/migrator/pipeline/source"
	"github.com/boyxs/train-go/webook/migrator/pipeline/transform"
	"github.com/boyxs/train-go/webook/migrator/repository"
	"github.com/boyxs/train-go/webook/migrator/service"
	"github.com/boyxs/train-go/webook/pkg/logger"
)

// ── 同包手写 stub ────────────────────────────────────────────
type stubTaskService struct {
	GetFn func(ctx context.Context, id int64) (domain.Task, error)
}

func (s *stubTaskService) Create(_ context.Context, _ service.CreateReq) (int64, error) {
	return 0, nil
}
func (s *stubTaskService) Get(ctx context.Context, id int64) (domain.Task, error) {
	return s.GetFn(ctx, id)
}
func (s *stubTaskService) UpdateStatus(_ context.Context, _ int64, _ domain.TaskStatus) error {
	return nil
}
func (s *stubTaskService) List(_ context.Context, _ repository.ListOpts) ([]domain.Task, int64, error) {
	return nil, 0, nil
}
func (s *stubTaskService) SetThrottle(_ context.Context, _ int64, _ domain.ThrottleConfig) error {
	return nil
}
func (s *stubTaskService) ClearThrottle(_ context.Context, _ int64) error { return nil }
func (s *stubTaskService) GetThrottle(_ context.Context, _ int64) (domain.ThrottleConfig, bool, error) {
	return domain.ThrottleConfig{}, false, nil
}

// stubSourceFactory 包装一个 stubSource：3 个 Build 方法都返回同一实例（FullEngine 测试只用 BuildFullSrc）。
type stubSourceFactory struct {
	src source.FullSource
}

func (f *stubSourceFactory) BuildFullSrc(_ context.Context, _ domain.Task, _ int) (source.FullSource, error) {
	return f.src, nil
}
func (f *stubSourceFactory) BuildIncrSrc(_ context.Context, _ domain.Task, _ int) (source.IncrSource, error) {
	return nil, nil
}
func (f *stubSourceFactory) BuildDst(_ context.Context, _ domain.Task, _ int) (source.FullSource, error) {
	return f.src, nil
}

// stubSinkFactory 同上。
type stubSinkFactory struct {
	snk sink.Sink
}

func (f *stubSinkFactory) BuildSrc(_ context.Context, _ domain.Task, _ int) (sink.Sink, error) {
	return f.snk, nil
}
func (f *stubSinkFactory) BuildDst(_ context.Context, _ domain.Task, _ int) (sink.Sink, error) {
	return f.snk, nil
}

// 测试用默认 task：TablesJSON 含合法 src/dst（factory MVP 解析需要）
func taskOK(id int64) domain.Task {
	return domain.Task{Id: id, Name: "t1", TablesJSON: `[{"src":"article","dst":"article_v1","partitionKey":"id"}]`}
}

type stubCheckpointRepository struct {
	SaveFn func(ctx context.Context, c domain.Checkpoint) error
}

func (s *stubCheckpointRepository) Save(ctx context.Context, c domain.Checkpoint) error {
	if s.SaveFn != nil {
		return s.SaveFn(ctx, c)
	}
	return nil
}
func (s *stubCheckpointRepository) ListByTaskPhase(_ context.Context, _ int64, _ string) ([]domain.Checkpoint, error) {
	return nil, nil
}

type stubSource struct {
	FullScanFn func(ctx context.Context, shard source.ShardSpec, out chan<- source.Row) error
}

func (s *stubSource) FullScan(ctx context.Context, sh source.ShardSpec, out chan<- source.Row) error {
	return s.FullScanFn(ctx, sh, out)
}
func (s *stubSource) Close() error { return nil }

type stubSink struct {
	ApplyFn func(ctx context.Context, batch []sink.Mutation) error
}

func (s *stubSink) Apply(ctx context.Context, batch []sink.Mutation) error {
	return s.ApplyFn(ctx, batch)
}
func (s *stubSink) Close() error { return nil }

// ── 测试 ─────────────────────────────────────────────────────

func newEngine(t *testing.T, ts service.TaskService, cd repository.CheckpointRepository,
	src source.FullSource, snk sink.Sink, cfg Config) FullEngine {
	t.Helper()
	return newEngineWithReg(t, ts, cd, src, snk, cfg, transform.NewRegistry())
}

func newEngineWithReg(t *testing.T, ts service.TaskService, cd repository.CheckpointRepository,
	src source.FullSource, snk sink.Sink, cfg Config, reg *transform.Registry) FullEngine {
	t.Helper()
	return NewFullEngine(ts, cd,
		&stubSourceFactory{src: src},
		&stubSinkFactory{snk: snk},
		logger.NewNopLogger(), reg, cfg)
}

// markTransformer 测试用：拷贝 Cols 并加标记列（验证引擎确实把每条 Mutation 过了一遍 transformer）。
type markTransformer struct{}

func (markTransformer) Transform(in sink.Mutation) (sink.Mutation, error) {
	out := in
	out.Cols = map[string]any{"marked": "yes"}
	for k, v := range in.Cols {
		out.Cols[k] = v
	}
	return out, nil
}

func TestFullEngine_Transform(t *testing.T) {
	t.Run("按 tableMapping.Transform 名 resolve 并应用到每条 Mutation", func(t *testing.T) {
		taskDAO := &stubTaskService{GetFn: func(_ context.Context, id int64) (domain.Task, error) {
			return domain.Task{Id: id, Name: "t", TablesJSON: `[{"src":"u","dst":"u_v1","partitionKey":"id","transform":"mark"}]`}, nil
		}}
		ckptRepo := &stubCheckpointRepository{SaveFn: func(_ context.Context, _ domain.Checkpoint) error { return nil }}
		src := &stubSource{FullScanFn: func(_ context.Context, _ source.ShardSpec, out chan<- source.Row) error {
			out <- source.Row{Table: "u", PK: "1", Cols: map[string]any{"id": int64(1)}}
			return nil
		}}
		var captured []sink.Mutation
		snk := &stubSink{ApplyFn: func(_ context.Context, batch []sink.Mutation) error {
			captured = append(captured, batch...)
			return nil
		}}
		reg := transform.NewRegistry()
		reg.Register("mark", markTransformer{})
		eng := newEngineWithReg(t, taskDAO, ckptRepo, src, snk, Config{}, reg)
		err := eng.Run(context.Background(), 1, []source.ShardSpec{{No: 0, PKMin: 1, PKMax: 100}})
		require.NoError(t, err)
		require.Len(t, captured, 1)
		assert.Equal(t, "yes", captured[0].Cols["marked"])
	})

	t.Run("transform 名未注册 → Run 报错（不静默退化）", func(t *testing.T) {
		taskDAO := &stubTaskService{GetFn: func(_ context.Context, id int64) (domain.Task, error) {
			return domain.Task{Id: id, Name: "t", TablesJSON: `[{"src":"u","dst":"u_v1","partitionKey":"id","transform":"nope"}]`}, nil
		}}
		ckptRepo := &stubCheckpointRepository{SaveFn: func(_ context.Context, _ domain.Checkpoint) error { return nil }}
		src := &stubSource{FullScanFn: func(_ context.Context, _ source.ShardSpec, _ chan<- source.Row) error { return nil }}
		snk := &stubSink{ApplyFn: func(_ context.Context, _ []sink.Mutation) error { return nil }}
		eng := newEngine(t, taskDAO, ckptRepo, src, snk, Config{}) // 空 registry
		err := eng.Run(context.Background(), 1, []source.ShardSpec{{No: 0, PKMin: 1, PKMax: 100}})
		assert.ErrorContains(t, err, "nope")
	})
}

func TestFullEngine_NonNumericPK(t *testing.T) {
	// Mongo 等非数值 PK（ObjectID hex）：全量游标按「最后发出的 PK 字符串」推进，不做数值解析。
	taskDAO := &stubTaskService{GetFn: func(_ context.Context, id int64) (domain.Task, error) {
		return taskOK(id), nil
	}}
	var lastCursor string
	ckptRepo := &stubCheckpointRepository{SaveFn: func(_ context.Context, c domain.Checkpoint) error {
		lastCursor = c.CursorValue
		return nil
	}}
	src := &stubSource{FullScanFn: func(_ context.Context, _ source.ShardSpec, out chan<- source.Row) error {
		out <- source.Row{Table: "u", PK: "65a", Cols: map[string]any{"_id": "65a"}}
		out <- source.Row{Table: "u", PK: "65b", Cols: map[string]any{"_id": "65b"}}
		return nil
	}}
	snk := &stubSink{ApplyFn: func(_ context.Context, _ []sink.Mutation) error { return nil }}
	eng := newEngine(t, taskDAO, ckptRepo, src, snk, Config{})
	err := eng.Run(context.Background(), 42, []source.ShardSpec{{No: 0, PKMin: 1, PKMax: 1 << 62}})
	require.NoError(t, err)
	assert.Equal(t, "65b", lastCursor) // 最后发出的 PK
}

func TestFullEngine_Run(t *testing.T) {
	t.Run("单 shard 跑通：3 行 → 1 批 Sink.Apply + checkpoint 更新", func(t *testing.T) {
		taskDAO := &stubTaskService{
			GetFn: func(_ context.Context, id int64) (domain.Task, error) {
				return taskOK(id), nil
			},
		}
		var ckptCalls int32
		ckptRepo := &stubCheckpointRepository{
			SaveFn: func(_ context.Context, c domain.Checkpoint) error {
				atomic.AddInt32(&ckptCalls, 1)
				assert.Equal(t, int64(42), c.TaskId)
				assert.Equal(t, "full", c.Phase)
				assert.Equal(t, "3", c.CursorValue) // lastPK=3
				return nil
			},
		}
		src := &stubSource{
			FullScanFn: func(_ context.Context, sh source.ShardSpec, out chan<- source.Row) error {
				out <- source.Row{Table: "article", PK: "1", Cols: map[string]any{"id": int64(1)}}
				out <- source.Row{Table: "article", PK: "2", Cols: map[string]any{"id": int64(2)}}
				out <- source.Row{Table: "article", PK: "3", Cols: map[string]any{"id": int64(3)}}
				return nil
			},
		}
		var sinkCalls int32
		snk := &stubSink{
			ApplyFn: func(_ context.Context, batch []sink.Mutation) error {
				atomic.AddInt32(&sinkCalls, 1)
				assert.Len(t, batch, 3)
				return nil
			},
		}
		eng := newEngine(t, taskDAO, ckptRepo, src, snk, Config{BatchSize: 100})
		err := eng.Run(context.Background(), 42, []source.ShardSpec{{No: 0, PKMin: 1, PKMax: 100}})
		require.NoError(t, err)
		assert.Equal(t, int32(1), atomic.LoadInt32(&sinkCalls))
		assert.Equal(t, int32(1), atomic.LoadInt32(&ckptCalls))
	})

	t.Run("多 shard 并行：2 shards 各 2 行 → Sink.Apply 调 2 次", func(t *testing.T) {
		taskDAO := &stubTaskService{
			GetFn: func(_ context.Context, id int64) (domain.Task, error) {
				return taskOK(id), nil
			},
		}
		ckptRepo := &stubCheckpointRepository{
			SaveFn: func(_ context.Context, _ domain.Checkpoint) error {
				return nil
			},
		}
		src := &stubSource{
			FullScanFn: func(_ context.Context, sh source.ShardSpec, out chan<- source.Row) error {
				out <- source.Row{PK: strconv.FormatInt(sh.PKMin, 10)}
				out <- source.Row{PK: strconv.FormatInt(sh.PKMax, 10)}
				return nil
			},
		}
		var sinkCalls int32
		snk := &stubSink{
			ApplyFn: func(_ context.Context, _ []sink.Mutation) error {
				atomic.AddInt32(&sinkCalls, 1)
				return nil
			},
		}
		eng := newEngine(t, taskDAO, ckptRepo, src, snk, Config{BatchSize: 100})
		err := eng.Run(context.Background(), 1, []source.ShardSpec{
			{No: 0, PKMin: 1, PKMax: 50},
			{No: 1, PKMin: 51, PKMax: 100},
		})
		require.NoError(t, err)
		assert.Equal(t, int32(2), atomic.LoadInt32(&sinkCalls))
	})

	t.Run("BatchSize 触发分批：5 行 + BatchSize=2 → 3 次 Sink.Apply", func(t *testing.T) {
		taskDAO := &stubTaskService{GetFn: func(_ context.Context, id int64) (domain.Task, error) {
			return taskOK(id), nil
		}}
		ckptRepo := &stubCheckpointRepository{SaveFn: func(_ context.Context, _ domain.Checkpoint) error { return nil }}
		src := &stubSource{
			FullScanFn: func(_ context.Context, _ source.ShardSpec, out chan<- source.Row) error {
				for i := int64(1); i <= 5; i++ {
					out <- source.Row{PK: strconv.FormatInt(i, 10)}
				}
				return nil
			},
		}
		var calls int32
		snk := &stubSink{ApplyFn: func(_ context.Context, _ []sink.Mutation) error {
			atomic.AddInt32(&calls, 1)
			return nil
		}}
		eng := newEngine(t, taskDAO, ckptRepo, src, snk, Config{BatchSize: 2})
		err := eng.Run(context.Background(), 1, []source.ShardSpec{{No: 0, PKMin: 1, PKMax: 10}})
		require.NoError(t, err)
		// 5 行 / batch=2 → 2 + 2 + 1 = 3 次
		assert.Equal(t, int32(3), atomic.LoadInt32(&calls))
	})

	t.Run("空 hintShards → 引擎按表自动 PKRange 切片", func(t *testing.T) {
		taskDAO := &stubTaskService{GetFn: func(_ context.Context, id int64) (domain.Task, error) {
			return taskOK(id), nil
		}}
		ckptRepo := &stubCheckpointRepository{SaveFn: func(_ context.Context, _ domain.Checkpoint) error { return nil }}
		// 空 src （PKRanger 未实现）→ 引擎兜底单 shard [1, 1<<62]；stub FullScan 立即返回 nil
		src := &stubSource{FullScanFn: func(_ context.Context, _ source.ShardSpec, _ chan<- source.Row) error {
			return nil
		}}
		snk := &stubSink{ApplyFn: func(_ context.Context, _ []sink.Mutation) error { return nil }}
		eng := newEngine(t, taskDAO, ckptRepo, src, snk, Config{})
		err := eng.Run(context.Background(), 1, nil)
		require.NoError(t, err)
	})

	t.Run("task 不存在 → error", func(t *testing.T) {
		boom := errors.New("not found")
		taskDAO := &stubTaskService{GetFn: func(_ context.Context, _ int64) (domain.Task, error) {
			return domain.Task{}, boom
		}}
		eng := newEngine(t, taskDAO, &stubCheckpointRepository{}, &stubSource{}, &stubSink{}, Config{})
		err := eng.Run(context.Background(), 1, []source.ShardSpec{{No: 0, PKMin: 1, PKMax: 10}})
		assert.ErrorIs(t, err, boom)
	})

	t.Run("Sink.Apply 失败 → Run 返回 error", func(t *testing.T) {
		taskDAO := &stubTaskService{GetFn: func(_ context.Context, id int64) (domain.Task, error) {
			return taskOK(id), nil
		}}
		src := &stubSource{FullScanFn: func(_ context.Context, _ source.ShardSpec, out chan<- source.Row) error {
			out <- source.Row{PK: "1"}
			return nil
		}}
		boom := errors.New("sink offline")
		snk := &stubSink{ApplyFn: func(_ context.Context, _ []sink.Mutation) error { return boom }}
		eng := newEngine(t, taskDAO, &stubCheckpointRepository{}, src, snk, Config{BatchSize: 1})
		err := eng.Run(context.Background(), 1, []source.ShardSpec{{No: 0, PKMin: 1, PKMax: 10}})
		assert.ErrorIs(t, err, boom)
	})

	t.Run("Pause 取消正在跑的 task", func(t *testing.T) {
		taskDAO := &stubTaskService{GetFn: func(_ context.Context, id int64) (domain.Task, error) {
			return taskOK(id), nil
		}}
		// Source 阻塞直到 ctx cancel
		src := &stubSource{FullScanFn: func(ctx context.Context, _ source.ShardSpec, _ chan<- source.Row) error {
			<-ctx.Done()
			return ctx.Err()
		}}
		eng := newEngine(t, taskDAO, &stubCheckpointRepository{}, src, &stubSink{}, Config{})
		done := make(chan error, 1)
		go func() {
			done <- eng.Run(context.Background(), 99, []source.ShardSpec{{No: 0, PKMin: 1, PKMax: 10}})
		}()
		// 让 Run 进入 errgroup.Wait()
		time.Sleep(50 * time.Millisecond)
		require.NoError(t, eng.Pause(99))
		select {
		case err := <-done:
			assert.Error(t, err, "Run should return ctx.Err after Pause")
		case <-time.After(500 * time.Millisecond):
			t.Fatal("Pause did not stop Run within 500ms")
		}
	})

	t.Run("Pause 不存在的 task → error", func(t *testing.T) {
		eng := newEngine(t, &stubTaskService{}, &stubCheckpointRepository{}, &stubSource{}, &stubSink{}, Config{})
		assert.Error(t, eng.Pause(404))
	})
}

func TestPlanShards(t *testing.T) {
	t.Run("3 等分", func(t *testing.T) {
		got := PlanShards(1, 30, 3)
		require.Len(t, got, 3)
		assert.Equal(t, int64(1), got[0].PKMin)
		assert.Equal(t, int64(10), got[0].PKMax)
		assert.Equal(t, int64(11), got[1].PKMin)
		assert.Equal(t, int64(20), got[1].PKMax)
		assert.Equal(t, int64(21), got[2].PKMin)
		assert.Equal(t, int64(30), got[2].PKMax) // 最后一片含尾
	})
	t.Run("count > 行数 → clamp 到行数", func(t *testing.T) {
		got := PlanShards(1, 3, 10)
		assert.Len(t, got, 3)
	})
	t.Run("max < min → 空 slice", func(t *testing.T) {
		assert.Nil(t, PlanShards(10, 5, 3))
	})
	t.Run("count=0 → 1 片包全部", func(t *testing.T) {
		got := PlanShards(1, 100, 0)
		assert.Len(t, got, 1)
		assert.Equal(t, int64(100), got[0].PKMax)
	})
}
