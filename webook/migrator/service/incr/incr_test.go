package incr

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/boyxs/train-go/webook/migrator/consts"
	"github.com/boyxs/train-go/webook/migrator/domain"
	"github.com/boyxs/train-go/webook/migrator/pipeline/sink"
	"github.com/boyxs/train-go/webook/migrator/pipeline/source"
	"github.com/boyxs/train-go/webook/migrator/pipeline/transform"
	"github.com/boyxs/train-go/webook/migrator/repository"
	"github.com/boyxs/train-go/webook/migrator/service"
	"github.com/boyxs/train-go/webook/pkg/logger"
)

// ── 手写 stub（接口嵌入 + 只覆盖用到的方法）─────────────────
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

// stubSourceFactory / stubSinkFactory：把 stubSource / stubSink 包装成 factory（IncrEngine 测试用 BuildIncrSrc）。
type stubSourceFactory struct{ src source.IncrSource }

func (f *stubSourceFactory) BuildFullSrc(_ context.Context, _ domain.Task, _ int) (source.FullSource, error) {
	return nil, nil
}
func (f *stubSourceFactory) BuildIncrSrc(_ context.Context, _ domain.Task, _ int) (source.IncrSource, error) {
	return f.src, nil
}
func (f *stubSourceFactory) BuildDst(_ context.Context, _ domain.Task, _ int) (source.FullSource, error) {
	return nil, nil
}

type stubSinkFactory struct{ snk sink.Sink }

func (f *stubSinkFactory) BuildSrc(_ context.Context, _ domain.Task, _ int) (sink.Sink, error) {
	return f.snk, nil
}
func (f *stubSinkFactory) BuildDst(_ context.Context, _ domain.Task, _ int) (sink.Sink, error) {
	return f.snk, nil
}

type stubCheckpointRepository struct {
	ListFn func(ctx context.Context, taskId int64, phase string) ([]domain.Checkpoint, error)
	SaveFn func(ctx context.Context, c domain.Checkpoint) error
}

func (s *stubCheckpointRepository) ListByTaskPhase(ctx context.Context, taskId int64, phase string) ([]domain.Checkpoint, error) {
	if s.ListFn != nil {
		return s.ListFn(ctx, taskId, phase)
	}
	return nil, nil
}

func (s *stubCheckpointRepository) Save(ctx context.Context, c domain.Checkpoint) error {
	if s.SaveFn != nil {
		return s.SaveFn(ctx, c)
	}
	return nil
}

// stubSource — 基础版（不实现 LagReporter）
type stubSource struct {
	IncrSubscribeFn func(ctx context.Context, ckpt domain.Checkpoint, out chan<- source.ChangeEvent) error
}

func (s *stubSource) FullScan(_ context.Context, _ source.ShardSpec, _ chan<- source.Row) error {
	return nil
}
func (s *stubSource) IncrSubscribe(ctx context.Context, ckpt domain.Checkpoint, out chan<- source.ChangeEvent) error {
	return s.IncrSubscribeFn(ctx, ckpt, out)
}
func (s *stubSource) Close() error { return nil }

// stubSourceWithLag — 实现 LagReporter
type stubSourceWithLag struct {
	*stubSource
	LagFn func(taskId int64) int64
}

func (s *stubSourceWithLag) Lag(taskId int64) int64 { return s.LagFn(taskId) }

type stubSink struct {
	ApplyFn func(ctx context.Context, batch []sink.Mutation) error
}

func (s *stubSink) Apply(ctx context.Context, batch []sink.Mutation) error {
	return s.ApplyFn(ctx, batch)
}
func (s *stubSink) Close() error { return nil }

// ── tests ──────────────────────────────────────────────────

func newEngine(t *testing.T, ts service.TaskService, cd repository.CheckpointRepository,
	src source.IncrSource, snk sink.Sink, cfg Config) IncrEngine {
	t.Helper()
	return newEngineWithReg(t, ts, cd, src, snk, cfg, transform.NewRegistry())
}

func newEngineWithReg(t *testing.T, ts service.TaskService, cd repository.CheckpointRepository,
	src source.IncrSource, snk sink.Sink, cfg Config, reg *transform.Registry) IncrEngine {
	t.Helper()
	return NewIncrEngine(ts, cd,
		&stubSourceFactory{src: src},
		&stubSinkFactory{snk: snk},
		logger.NewNopLogger(), reg, cfg)
}

// markTransformer 测试用：拷 Cols 加标记列，验证 incr 引擎把每条 Mutation 过了 transformer。
type markTransformer struct{}

func (markTransformer) Transform(in sink.Mutation) (sink.Mutation, error) {
	out := in
	out.Cols = map[string]any{"marked": "yes"}
	for k, v := range in.Cols {
		out.Cols[k] = v
	}
	return out, nil
}

func TestIncrEngine_Transform(t *testing.T) {
	taskDAO := &stubTaskService{GetFn: func(_ context.Context, id int64) (domain.Task, error) {
		return domain.Task{Id: id, Name: "t", TablesJSON: `[{"src":"u","dst":"u_v1","partitionKey":"id","transform":"mark"}]`}, nil
	}}
	src := &stubSource{IncrSubscribeFn: func(ctx context.Context, _ domain.Checkpoint, out chan<- source.ChangeEvent) error {
		out <- source.ChangeEvent{Op: sink.OpInsert, Table: "u", PK: "1", After: map[string]any{"id": int64(1)}, BinlogPos: "f/1", EventTs: 1}
		<-ctx.Done()
		return ctx.Err()
	}}
	var mu sync.Mutex
	var captured []sink.Mutation
	snk := &stubSink{ApplyFn: func(_ context.Context, batch []sink.Mutation) error {
		mu.Lock()
		captured = append(captured, batch...)
		mu.Unlock()
		return nil
	}}
	reg := transform.NewRegistry()
	reg.Register("mark", markTransformer{})
	eng := newEngineWithReg(t, taskDAO, &stubCheckpointRepository{}, src, snk,
		Config{BatchSize: 1, PartitionCount: 1, FlushInterval: 10 * time.Millisecond}, reg)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- eng.Run(ctx, 1) }()
	time.Sleep(150 * time.Millisecond)
	cancel()
	<-done

	mu.Lock()
	defer mu.Unlock()
	require.NotEmpty(t, captured)
	assert.Equal(t, "yes", captured[0].Cols["marked"])
}

func taskOK(id int64) domain.Task {
	return domain.Task{Id: id, TablesJSON: `[{"src":"article","dst":"article_v1","partitionKey":"id"}]`}
}

func TestIncrEngine_Run(t *testing.T) {
	t.Run("3 事件 → 1 批 Sink.Apply + checkpoint 更新到最新 binlog pos", func(t *testing.T) {
		taskDAO := &stubTaskService{GetFn: func(_ context.Context, id int64) (domain.Task, error) {
			return taskOK(id), nil
		}}
		var ckptUpserts int32
		var capturedCursor string
		ckptRepo := &stubCheckpointRepository{
			SaveFn: func(_ context.Context, c domain.Checkpoint) error {
				atomic.AddInt32(&ckptUpserts, 1)
				capturedCursor = c.CursorValue
				assert.Equal(t, consts.PhaseIncr, c.Phase)
				assert.Equal(t, consts.CursorKindBinlog, c.CursorKind)
				return nil
			},
		}
		src := &stubSource{
			IncrSubscribeFn: func(ctx context.Context, _ domain.Checkpoint, out chan<- source.ChangeEvent) error {
				out <- source.ChangeEvent{Op: sink.OpInsert, Table: "article", PK: "1", After: map[string]any{"id": int64(1)}, BinlogPos: "mysql-bin.000001/4", EventTs: 1000}
				out <- source.ChangeEvent{Op: sink.OpUpdate, Table: "article", PK: "2", After: map[string]any{"id": int64(2)}, BinlogPos: "mysql-bin.000001/100", EventTs: 1500}
				out <- source.ChangeEvent{Op: sink.OpDelete, Table: "article", PK: "3", Before: map[string]any{"id": int64(3)}, BinlogPos: "mysql-bin.000001/200", EventTs: 2000}
				<-ctx.Done()
				return ctx.Err()
			},
		}
		var sinkCalls int32
		snk := &stubSink{
			ApplyFn: func(_ context.Context, batch []sink.Mutation) error {
				atomic.AddInt32(&sinkCalls, 1)
				assert.Len(t, batch, 3)
				// 检查 delete 事件 Cols 来自 Before
				assert.Equal(t, "3", batch[2].PK)
				assert.Equal(t, sink.OpDelete, batch[2].Op)
				return nil
			},
		}
		eng := newEngine(t, taskDAO, ckptRepo, src, snk, Config{BatchSize: 100})

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- eng.Run(ctx, 42) }()
		// 给 IncrSubscribe goroutine 时间推 3 个事件 + 主 goroutine 攒批
		time.Sleep(100 * time.Millisecond)
		cancel()
		require.NoError(t, <-done)
		assert.Equal(t, int32(1), atomic.LoadInt32(&sinkCalls))
		assert.Equal(t, int32(1), atomic.LoadInt32(&ckptUpserts))
		assert.Equal(t, "mysql-bin.000001/200", capturedCursor)
	})

	t.Run("BatchSize 触发分批：5 事件 + batch=2 → 3 次 Sink.Apply", func(t *testing.T) {
		taskDAO := &stubTaskService{GetFn: func(_ context.Context, id int64) (domain.Task, error) {
			return taskOK(id), nil
		}}
		ckptRepo := &stubCheckpointRepository{}
		src := &stubSource{
			IncrSubscribeFn: func(ctx context.Context, _ domain.Checkpoint, out chan<- source.ChangeEvent) error {
				for i := int64(1); i <= 5; i++ {
					out <- source.ChangeEvent{Op: sink.OpInsert, PK: strconv.FormatInt(i, 10), BinlogPos: "f/1", EventTs: i}
				}
				<-ctx.Done()
				return ctx.Err()
			},
		}
		var calls int32
		snk := &stubSink{ApplyFn: func(_ context.Context, _ []sink.Mutation) error {
			atomic.AddInt32(&calls, 1)
			return nil
		}}
		eng := newEngine(t, taskDAO, ckptRepo, src, snk, Config{BatchSize: 2})

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- eng.Run(ctx, 1) }()
		time.Sleep(100 * time.Millisecond)
		cancel()
		require.NoError(t, <-done)
		// 5 / batch=2 → 2 + 2 + 1 = 3
		assert.Equal(t, int32(3), atomic.LoadInt32(&calls))
	})

	t.Run("Resume from existing checkpoint", func(t *testing.T) {
		taskDAO := &stubTaskService{GetFn: func(_ context.Context, id int64) (domain.Task, error) {
			return taskOK(id), nil
		}}
		ckptRepo := &stubCheckpointRepository{
			ListFn: func(_ context.Context, _ int64, _ string) ([]domain.Checkpoint, error) {
				return []domain.Checkpoint{{ShardNo: 0, CursorKind: consts.CursorKindBinlog, CursorValue: "mysql-bin.000003/4096"}}, nil
			},
		}
		var capturedCkpt domain.Checkpoint
		src := &stubSource{
			IncrSubscribeFn: func(ctx context.Context, ckpt domain.Checkpoint, _ chan<- source.ChangeEvent) error {
				capturedCkpt = ckpt
				<-ctx.Done()
				return ctx.Err()
			},
		}
		eng := newEngine(t, taskDAO, ckptRepo, src, &stubSink{}, Config{})

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- eng.Run(ctx, 1) }()
		time.Sleep(50 * time.Millisecond)
		cancel()
		require.NoError(t, <-done)
		assert.Equal(t, "mysql-bin.000003/4096", capturedCkpt.CursorValue)
		assert.Equal(t, consts.CursorKindBinlog, capturedCkpt.CursorKind)
	})

	t.Run("task 不存在 → error", func(t *testing.T) {
		boom := errors.New("not found")
		taskDAO := &stubTaskService{GetFn: func(_ context.Context, _ int64) (domain.Task, error) {
			return domain.Task{}, boom
		}}
		eng := newEngine(t, taskDAO, &stubCheckpointRepository{}, &stubSource{}, &stubSink{}, Config{})
		err := eng.Run(context.Background(), 1)
		assert.ErrorIs(t, err, boom)
	})

	t.Run("Sink.Apply 失败 → Run 返回 error", func(t *testing.T) {
		taskDAO := &stubTaskService{GetFn: func(_ context.Context, id int64) (domain.Task, error) {
			return taskOK(id), nil
		}}
		src := &stubSource{
			IncrSubscribeFn: func(_ context.Context, _ domain.Checkpoint, out chan<- source.ChangeEvent) error {
				out <- source.ChangeEvent{Op: sink.OpInsert, PK: "1", BinlogPos: "f/1"}
				return nil
			},
		}
		boom := errors.New("sink offline")
		snk := &stubSink{ApplyFn: func(_ context.Context, _ []sink.Mutation) error { return boom }}
		eng := newEngine(t, taskDAO, &stubCheckpointRepository{}, src, snk, Config{BatchSize: 1})
		err := eng.Run(context.Background(), 1)
		assert.ErrorIs(t, err, boom)
	})

	t.Run("Pause 取消正在跑的 task", func(t *testing.T) {
		taskDAO := &stubTaskService{GetFn: func(_ context.Context, id int64) (domain.Task, error) {
			return taskOK(id), nil
		}}
		src := &stubSource{
			IncrSubscribeFn: func(ctx context.Context, _ domain.Checkpoint, _ chan<- source.ChangeEvent) error {
				<-ctx.Done()
				return ctx.Err()
			},
		}
		eng := newEngine(t, taskDAO, &stubCheckpointRepository{}, src, &stubSink{}, Config{})
		done := make(chan error, 1)
		go func() { done <- eng.Run(context.Background(), 99) }()
		time.Sleep(50 * time.Millisecond)
		require.NoError(t, eng.Pause(99))
		select {
		case err := <-done:
			assert.NoError(t, err, "Pause should be graceful exit, not error")
		case <-time.After(500 * time.Millisecond):
			t.Fatal("Pause did not stop Run within 500ms")
		}
	})

	t.Run("Pause 不存在的 task → error", func(t *testing.T) {
		eng := newEngine(t, &stubTaskService{}, &stubCheckpointRepository{}, &stubSource{}, &stubSink{}, Config{})
		assert.Error(t, eng.Pause(404))
	})
}

func TestIncrEngine_PartitionResume(t *testing.T) {
	t.Run("多 partition 不同 ckpt → IncrSubscribe 从 min 起", func(t *testing.T) {
		taskDAO := &stubTaskService{GetFn: func(_ context.Context, id int64) (domain.Task, error) {
			return taskOK(id), nil
		}}
		// partition 0 已经处理到 pos=100，partition 1 停在 pos=50；min=50
		ckptRepo := &stubCheckpointRepository{
			ListFn: func(_ context.Context, _ int64, _ string) ([]domain.Checkpoint, error) {
				return []domain.Checkpoint{
					{ShardNo: 0, CursorKind: consts.CursorKindBinlog, CursorValue: "mysql-bin.000001/100"},
					{ShardNo: 1, CursorKind: consts.CursorKindBinlog, CursorValue: "mysql-bin.000001/50"},
				}, nil
			},
		}
		var capturedCkpt domain.Checkpoint
		src := &stubSource{
			IncrSubscribeFn: func(ctx context.Context, ckpt domain.Checkpoint, _ chan<- source.ChangeEvent) error {
				capturedCkpt = ckpt
				<-ctx.Done()
				return ctx.Err()
			},
		}
		eng := newEngine(t, taskDAO, ckptRepo, src, &stubSink{}, Config{PartitionCount: 2})

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- eng.Run(ctx, 1) }()
		time.Sleep(50 * time.Millisecond)
		cancel()
		require.NoError(t, <-done)
		assert.Equal(t, "mysql-bin.000001/50", capturedCkpt.CursorValue,
			"应选 partition 1 的 pos=50 而非 partition 0 的 pos=100")
	})

	t.Run("有 partition 从未 flush → 用空 cursor 起点", func(t *testing.T) {
		taskDAO := &stubTaskService{GetFn: func(_ context.Context, id int64) (domain.Task, error) {
			return taskOK(id), nil
		}}
		// partition 0 已经处理到 pos=200；partition 1 从未 flush（DB 无记录）
		ckptRepo := &stubCheckpointRepository{
			ListFn: func(_ context.Context, _ int64, _ string) ([]domain.Checkpoint, error) {
				return []domain.Checkpoint{
					{ShardNo: 0, CursorKind: consts.CursorKindBinlog, CursorValue: "mysql-bin.000001/200"},
				}, nil
			},
		}
		var capturedCkpt domain.Checkpoint
		src := &stubSource{
			IncrSubscribeFn: func(ctx context.Context, ckpt domain.Checkpoint, _ chan<- source.ChangeEvent) error {
				capturedCkpt = ckpt
				<-ctx.Done()
				return ctx.Err()
			},
		}
		eng := newEngine(t, taskDAO, ckptRepo, src, &stubSink{}, Config{PartitionCount: 2})

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- eng.Run(ctx, 1) }()
		time.Sleep(50 * time.Millisecond)
		cancel()
		require.NoError(t, <-done)
		assert.Empty(t, capturedCkpt.CursorValue,
			"partition 1 没 ckpt → 必须从最早起点订阅，不能从 partition 0 的 200 起跳")
	})

	t.Run("ckpt 不回退：重订阅事件位点 < startPos 时不写 ckpt", func(t *testing.T) {
		taskDAO := &stubTaskService{GetFn: func(_ context.Context, id int64) (domain.Task, error) {
			return taskOK(id), nil
		}}
		// partition 0 启动时 startPos=100
		ckptRepo := &stubCheckpointRepository{
			ListFn: func(_ context.Context, _ int64, _ string) ([]domain.Checkpoint, error) {
				return []domain.Checkpoint{
					{ShardNo: 0, CursorKind: consts.CursorKindBinlog, CursorValue: "mysql-bin.000001/100"},
				}, nil
			},
		}
		var ckptUpserts int32
		var lastCkptValue string
		ckptRepo.SaveFn = func(_ context.Context, c domain.Checkpoint) error {
			atomic.AddInt32(&ckptUpserts, 1)
			lastCkptValue = c.CursorValue
			return nil
		}
		// 推 2 个事件：pos=50（< startPos，重订阅事件），pos=60（仍 < startPos）
		src := &stubSource{
			IncrSubscribeFn: func(ctx context.Context, _ domain.Checkpoint, out chan<- source.ChangeEvent) error {
				out <- source.ChangeEvent{Op: sink.OpInsert, PK: "1", BinlogPos: "mysql-bin.000001/50", EventTs: 1}
				out <- source.ChangeEvent{Op: sink.OpInsert, PK: "1", BinlogPos: "mysql-bin.000001/60", EventTs: 2}
				<-ctx.Done()
				return ctx.Err()
			},
		}
		snk := &stubSink{ApplyFn: func(_ context.Context, _ []sink.Mutation) error { return nil }}
		eng := newEngine(t, taskDAO, ckptRepo, src, snk, Config{BatchSize: 1, PartitionCount: 1})

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- eng.Run(ctx, 1) }()
		time.Sleep(100 * time.Millisecond)
		cancel()
		require.NoError(t, <-done)
		assert.Equal(t, int32(0), atomic.LoadInt32(&ckptUpserts),
			"位点 50 / 60 均 < startPos 100，不应触发 ckpt 写入")
		assert.Empty(t, lastCkptValue)
	})

	t.Run("ckpt 可前进：事件位点 > startPos 时正常写", func(t *testing.T) {
		taskDAO := &stubTaskService{GetFn: func(_ context.Context, id int64) (domain.Task, error) {
			return taskOK(id), nil
		}}
		ckptRepo := &stubCheckpointRepository{
			ListFn: func(_ context.Context, _ int64, _ string) ([]domain.Checkpoint, error) {
				return []domain.Checkpoint{
					{ShardNo: 0, CursorKind: consts.CursorKindBinlog, CursorValue: "mysql-bin.000001/100"},
				}, nil
			},
		}
		var lastCkptValue string
		ckptRepo.SaveFn = func(_ context.Context, c domain.Checkpoint) error {
			lastCkptValue = c.CursorValue
			return nil
		}
		src := &stubSource{
			IncrSubscribeFn: func(ctx context.Context, _ domain.Checkpoint, out chan<- source.ChangeEvent) error {
				out <- source.ChangeEvent{Op: sink.OpInsert, PK: "1", BinlogPos: "mysql-bin.000001/200"}
				<-ctx.Done()
				return ctx.Err()
			},
		}
		snk := &stubSink{ApplyFn: func(_ context.Context, _ []sink.Mutation) error { return nil }}
		eng := newEngine(t, taskDAO, ckptRepo, src, snk, Config{BatchSize: 1, PartitionCount: 1})

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- eng.Run(ctx, 1) }()
		time.Sleep(100 * time.Millisecond)
		cancel()
		require.NoError(t, <-done)
		assert.Equal(t, "mysql-bin.000001/200", lastCkptValue, "位点 200 > startPos 100 应正常写 ckpt")
	})
}

func TestCompareBinlogPos(t *testing.T) {
	t.Run("同 file 比 pos", func(t *testing.T) {
		assert.Equal(t, -1, compareBinlogPos("mysql-bin.000001/50", "mysql-bin.000001/100"))
		assert.Equal(t, 1, compareBinlogPos("mysql-bin.000001/100", "mysql-bin.000001/50"))
		assert.Equal(t, 0, compareBinlogPos("mysql-bin.000001/100", "mysql-bin.000001/100"))
	})
	t.Run("不同 file 比 file 序", func(t *testing.T) {
		assert.Equal(t, -1, compareBinlogPos("mysql-bin.000001/9999", "mysql-bin.000002/0"))
		assert.Equal(t, 1, compareBinlogPos("mysql-bin.000003/0", "mysql-bin.000002/9999"))
	})
	t.Run("数字 pos 字典序陷阱：100 vs 99 不能字典序", func(t *testing.T) {
		// "mysql-bin.000001/100" 字典序 < "mysql-bin.000001/99"（'1' < '9'），但数字 100 > 99
		assert.Equal(t, 1, compareBinlogPos("mysql-bin.000001/100", "mysql-bin.000001/99"))
	})
	t.Run("malformed 回退字符串比较", func(t *testing.T) {
		assert.Equal(t, -1, compareBinlogPos("aaa", "bbb"))
		assert.Equal(t, 0, compareBinlogPos("aaa", "aaa"))
	})
}

func TestMinPartitionCkpt(t *testing.T) {
	t.Run("选 pos 最小的", func(t *testing.T) {
		ckpts := []domain.Checkpoint{
			{ShardNo: 0, CursorValue: "mysql-bin.000001/100"},
			{ShardNo: 1, CursorValue: "mysql-bin.000001/50"},
			{ShardNo: 2, CursorValue: "mysql-bin.000001/200"},
		}
		got := minPartitionCkpt(ckpts)
		assert.Equal(t, int32(1), got.ShardNo)
		assert.Equal(t, "mysql-bin.000001/50", got.CursorValue)
	})
	t.Run("空 cursor 必返回（首次启动 partition）", func(t *testing.T) {
		ckpts := []domain.Checkpoint{
			{ShardNo: 0, CursorValue: "mysql-bin.000001/200"},
			{ShardNo: 1, CursorValue: ""}, // 从未 flush
			{ShardNo: 2, CursorValue: "mysql-bin.000001/100"},
		}
		got := minPartitionCkpt(ckpts)
		assert.Empty(t, got.CursorValue, "空 cursor 比任何已写位点都早，必须返回它")
	})
	t.Run("空 slice", func(t *testing.T) {
		got := minPartitionCkpt(nil)
		assert.Empty(t, got.CursorValue)
	})
}

func TestIncrEngine_PartitionParallel(t *testing.T) {
	t.Run("PartitionCount=4 → 同 PK 始终落同一 partition + 多 partition 并行", func(t *testing.T) {
		taskDAO := &stubTaskService{GetFn: func(_ context.Context, id int64) (domain.Task, error) {
			return taskOK(id), nil
		}}
		var ckptShards sync.Map // partition → bool（记录哪些 partition 写过 checkpoint）
		ckptRepo := &stubCheckpointRepository{
			SaveFn: func(_ context.Context, c domain.Checkpoint) error {
				ckptShards.Store(int(c.ShardNo), true)
				return nil
			},
		}
		// Source 推 8 个不同 PK 的事件
		src := &stubSource{
			IncrSubscribeFn: func(ctx context.Context, _ domain.Checkpoint, out chan<- source.ChangeEvent) error {
				for i := int64(1); i <= 8; i++ {
					out <- source.ChangeEvent{
						Op: sink.OpInsert, PK: strconv.FormatInt(i, 10),
						BinlogPos: "f/" + strconv.FormatInt(i, 10),
						EventTs:   i,
					}
				}
				<-ctx.Done()
				return ctx.Err()
			},
		}
		// Sink 记录每条事件的 PK
		var batchMu sync.Mutex
		seenPKs := map[string]struct{}{}
		var applyCount int32
		snk := &stubSink{ApplyFn: func(_ context.Context, batch []sink.Mutation) error {
			atomic.AddInt32(&applyCount, 1)
			batchMu.Lock()
			for _, m := range batch {
				seenPKs[m.PK] = struct{}{}
			}
			batchMu.Unlock()
			return nil
		}}
		eng := newEngine(t, taskDAO, ckptRepo, src, snk, Config{
			BatchSize:      1, // 每条事件一批，确保每 partition 都 flush
			PartitionCount: 4,
		})

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- eng.Run(ctx, 1) }()
		// 给 dispatcher + workers 时间消费 8 个事件
		time.Sleep(150 * time.Millisecond)
		cancel()
		require.NoError(t, <-done)

		// 8 条事件全部 sink 写入
		batchMu.Lock()
		assert.Len(t, seenPKs, 8)
		batchMu.Unlock()

		// 至少 2 个 partition 写过 checkpoint（4 partition + 8 PK，FNV 分布应触多 partition）
		var partitionsWritten int
		ckptShards.Range(func(_, _ any) bool {
			partitionsWritten++
			return true
		})
		assert.GreaterOrEqual(t, partitionsWritten, 2,
			"应有多个 partition 写 checkpoint（说明真并行）")
	})

	t.Run("PartitionCount=1（默认） → 单 partition checkpoint shard_no=0", func(t *testing.T) {
		taskDAO := &stubTaskService{GetFn: func(_ context.Context, id int64) (domain.Task, error) {
			return taskOK(id), nil
		}}
		var capturedShardNo int32
		ckptRepo := &stubCheckpointRepository{
			SaveFn: func(_ context.Context, c domain.Checkpoint) error {
				capturedShardNo = c.ShardNo
				return nil
			},
		}
		src := &stubSource{
			IncrSubscribeFn: func(ctx context.Context, _ domain.Checkpoint, out chan<- source.ChangeEvent) error {
				out <- source.ChangeEvent{Op: sink.OpInsert, PK: "99", BinlogPos: "f/1"}
				<-ctx.Done()
				return ctx.Err()
			},
		}
		snk := &stubSink{ApplyFn: func(_ context.Context, _ []sink.Mutation) error { return nil }}
		eng := newEngine(t, taskDAO, ckptRepo, src, snk, Config{BatchSize: 1}) // PartitionCount=0 → 默认 1

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- eng.Run(ctx, 1) }()
		time.Sleep(100 * time.Millisecond)
		cancel()
		require.NoError(t, <-done)
		assert.Equal(t, int32(0), capturedShardNo)
	})
}

func TestPartitionOf(t *testing.T) {
	t.Run("n<=1 → 0", func(t *testing.T) {
		assert.Equal(t, 0, partitionOf("12345", 0))
		assert.Equal(t, 0, partitionOf("12345", 1))
	})
	t.Run("同 PK 多次调用结果一致", func(t *testing.T) {
		assert.Equal(t, partitionOf("12345", 4), partitionOf("12345", 4))
	})
	t.Run("不同 PK 大致均匀落 N partition", func(t *testing.T) {
		buckets := make(map[int]int)
		for i := int64(0); i < 1000; i++ {
			buckets[partitionOf(strconv.FormatInt(i, 10), 4)]++
		}
		// FNV 分布应每 bucket 都有，且不会全部落一个
		assert.Len(t, buckets, 4, "应该 4 个 partition 都有")
		for _, count := range buckets {
			assert.Greater(t, count, 100, "每个 partition 至少 100 个（n=1000/4=250±20% 范围）")
		}
	})
	t.Run("结果范围 [0, n)", func(t *testing.T) {
		for i := int64(0); i < 1000; i++ {
			p := partitionOf(strconv.FormatInt(i, 10), 8)
			assert.GreaterOrEqual(t, p, 0)
			assert.Less(t, p, 8)
		}
	})
}

func TestIncrEngine_Lag(t *testing.T) {
	// Lag 复用 Run 期间 factory build 的 Source（runningSources 缓存）；
	// 所以测试必须先启动 Run 让 src 进 cache 再调 Lag。
	runAndLag := func(t *testing.T, src source.IncrSource) (int64, error) {
		t.Helper()
		taskSvc := &stubTaskService{GetFn: func(_ context.Context, id int64) (domain.Task, error) {
			return taskOK(id), nil
		}}
		eng := newEngine(t, taskSvc, &stubCheckpointRepository{}, src, &stubSink{}, Config{})
		go func() { _ = eng.Run(context.Background(), 7) }()
		// 等 Run 启动 + 写入 runningSources
		time.Sleep(30 * time.Millisecond)
		lag, err := eng.Lag(7)
		_ = eng.Pause(7)
		return lag, err
	}

	t.Run("Source 实现 LagReporter → 返回值", func(t *testing.T) {
		src := &stubSourceWithLag{
			stubSource: &stubSource{IncrSubscribeFn: func(ctx context.Context, _ domain.Checkpoint, _ chan<- source.ChangeEvent) error {
				<-ctx.Done()
				return ctx.Err()
			}},
			LagFn: func(_ int64) int64 { return 1234 },
		}
		lag, err := runAndLag(t, src)
		require.NoError(t, err)
		assert.Equal(t, int64(1234), lag)
	})

	t.Run("Source 不实现 LagReporter → error", func(t *testing.T) {
		src := &stubSource{IncrSubscribeFn: func(ctx context.Context, _ domain.Checkpoint, _ chan<- source.ChangeEvent) error {
			<-ctx.Done()
			return ctx.Err()
		}}
		_, err := runAndLag(t, src)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "LagReporter")
	})

	t.Run("task 未跑 → not running error", func(t *testing.T) {
		eng := newEngine(t, &stubTaskService{}, &stubCheckpointRepository{}, &stubSource{}, &stubSink{}, Config{})
		_, err := eng.Lag(999)
		assert.ErrorContains(t, err, "not running")
	})
}

func TestChangeToMutation(t *testing.T) {
	t.Run("insert / update → Cols=After", func(t *testing.T) {
		m := changeToMutation(source.ChangeEvent{
			Op: sink.OpUpdate, Table: "article", PK: "1",
			Before: map[string]any{"title": "old"}, After: map[string]any{"title": "new"},
			EventTs: 1234,
		})
		assert.Equal(t, sink.OpUpdate, m.Op)
		assert.Equal(t, "1", m.PK)
		assert.Equal(t, "new", m.Cols["title"])
		assert.Equal(t, int64(1234), m.Version)
	})

	t.Run("delete → Cols=Before", func(t *testing.T) {
		m := changeToMutation(source.ChangeEvent{
			Op: sink.OpDelete, Table: "article", PK: "9",
			Before: map[string]any{"title": "gone"},
		})
		assert.Equal(t, sink.OpDelete, m.Op)
		assert.Equal(t, "gone", m.Cols["title"])
	})
}
