package verify

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/webook/migrator/consts"
	"github.com/webook/migrator/domain"
	"github.com/webook/migrator/pipeline/sink"
	"github.com/webook/migrator/pipeline/source"
	"github.com/webook/migrator/pipeline/transform"
	"github.com/webook/migrator/repository"
	"github.com/webook/migrator/service"
	"github.com/webook/pkg/logger"
)

// ── stub service / Source / Sink（接口嵌入 + 覆盖用到的方法）──────────
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

// stubSourceFactory / stubSinkFactory：BuildFullSrc / BuildIncrSrc / BuildDst 分别可注入不同实例。
type stubSourceFactory struct{ src, dst source.FullSource }

func (f *stubSourceFactory) BuildFullSrc(_ context.Context, _ domain.Task, _ int) (source.FullSource, error) {
	return f.src, nil
}
func (f *stubSourceFactory) BuildIncrSrc(_ context.Context, _ domain.Task, _ int) (source.IncrSource, error) {
	return nil, nil
}
func (f *stubSourceFactory) BuildDst(_ context.Context, _ domain.Task, _ int) (source.FullSource, error) {
	return f.dst, nil
}

type stubSinkFactory struct{ srcSnk, dstSnk sink.Sink }

func (f *stubSinkFactory) BuildSrc(_ context.Context, _ domain.Task, _ int) (sink.Sink, error) {
	return f.srcSnk, nil
}
func (f *stubSinkFactory) BuildDst(_ context.Context, _ domain.Task, _ int) (sink.Sink, error) {
	return f.dstSnk, nil
}

type stubValidateLogRepository struct {
	BatchInsertFn  func(ctx context.Context, logs []domain.ValidateLog) error
	MarkRepairedFn func(ctx context.Context, ids []int64) error
	FindByIDsFn    func(ctx context.Context, ids []int64) ([]domain.ValidateLog, error)
}

func (s *stubValidateLogRepository) BatchInsert(ctx context.Context, logs []domain.ValidateLog) error {
	if s.BatchInsertFn != nil {
		return s.BatchInsertFn(ctx, logs)
	}
	return nil
}

func (s *stubValidateLogRepository) ListUnrepaired(_ context.Context, _ int64, _, _ int) ([]domain.ValidateLog, int64, error) {
	return nil, 0, nil
}

func (s *stubValidateLogRepository) MarkRepaired(ctx context.Context, ids []int64) error {
	if s.MarkRepairedFn != nil {
		return s.MarkRepairedFn(ctx, ids)
	}
	return nil
}

func (s *stubValidateLogRepository) FindByIDs(ctx context.Context, ids []int64) ([]domain.ValidateLog, error) {
	if s.FindByIDsFn != nil {
		return s.FindByIDsFn(ctx, ids)
	}
	return nil, nil
}

// stubSinkForRepair — minimal Sink stub for repair overwrite tests
type stubSinkForRepair struct {
	ApplyFn func(ctx context.Context, batch []sink.Mutation) error
}

func (s *stubSinkForRepair) Apply(ctx context.Context, batch []sink.Mutation) error {
	if s.ApplyFn != nil {
		return s.ApplyFn(ctx, batch)
	}
	return nil
}
func (s *stubSinkForRepair) Close() error { return nil }

type stubSource struct {
	Rows []source.Row
}

func (s *stubSource) FullScan(_ context.Context, _ source.ShardSpec, out chan<- source.Row) error {
	for _, r := range s.Rows {
		out <- r
	}
	return nil
}
func (s *stubSource) Close() error { return nil }

func newEngine(t *testing.T, ts service.TaskService, vd repository.ValidateLogRepository,
	src, dst source.FullSource, cfg Config) VerifyEngine {
	t.Helper()
	return newEngineWithReg(t, ts, vd, src, dst, cfg, transform.NewRegistry())
}

func newEngineWithReg(t *testing.T, ts service.TaskService, vd repository.ValidateLogRepository,
	src, dst source.FullSource, cfg Config, reg *transform.Registry) VerifyEngine {
	t.Helper()
	return NewVerifyEngine(ts, vd,
		&stubSourceFactory{src: src, dst: dst},
		&stubSinkFactory{},
		logger.NewNopLogger(), reg, cfg)
}

// TestVerifyEngine_HeterogeneousTransform 验证异构对账：源是文档形态（profile 嵌套 map），
// 目标是关系形态（profile JSON 字符串）；带 mongo_to_relational transform → 比对前两侧归一 → 0 mismatch。
func TestVerifyEngine_HeterogeneousTransform(t *testing.T) {
	td := &stubTaskService{GetFn: func(_ context.Context, id int64) (domain.Task, error) {
		return domain.Task{Id: id, TablesJSON: `[{"src":"u","dst":"u","partitionKey":"_id","transform":"mongo_to_relational"}]`}, nil
	}}
	var inserted []domain.ValidateLog
	vd := &stubValidateLogRepository{BatchInsertFn: func(_ context.Context, logs []domain.ValidateLog) error {
		inserted = logs
		return nil
	}}
	// 源：嵌套 map；目标：已拍平的 JSON 字符串（= 迁移时 transform 的产物）
	src := &stubSource{Rows: []source.Row{
		{PK: "1", Cols: map[string]any{"_id": "1", "name": "a", "profile": map[string]any{"city": "SG"}}},
	}}
	dst := &stubSource{Rows: []source.Row{
		{PK: "1", Cols: map[string]any{"_id": "1", "name": "a", "profile": `{"city":"SG"}`}},
	}}
	reg := transform.NewRegistry()
	reg.Register(transform.TransformMongoToRelational, transform.MongoToRelationalTransformer{})
	eng := newEngineWithReg(t, td, vd, src, dst, Config{}, reg)

	got, err := eng.Full(context.Background(), 1)
	require.NoError(t, err)
	assert.Equal(t, int64(0), got) // 归一后两侧一致，无差异（不带 transform 会把 map vs string 误报 diff）
	assert.Empty(t, inserted)
}

// newEngineWithSinks 用于 Repair overwrite 测试（注入 srcSink / dstSink）。
func newEngineWithSinks(t *testing.T, ts service.TaskService, vd repository.ValidateLogRepository,
	src, dst source.FullSource, srcSnk, dstSnk sink.Sink, cfg Config) VerifyEngine {
	t.Helper()
	return NewVerifyEngine(ts, vd,
		&stubSourceFactory{src: src, dst: dst},
		&stubSinkFactory{srcSnk: srcSnk, dstSnk: dstSnk},
		logger.NewNopLogger(), transform.NewRegistry(), cfg)
}

func taskOK(id int64) domain.Task {
	return domain.Task{Id: id, TablesJSON: `[{"src":"article","dst":"article_v1","partitionKey":"id"}]`}
}

func TestVerifyEngine_Full(t *testing.T) {
	t.Run("无差异 → 0 mismatch + 空 log", func(t *testing.T) {
		td := &stubTaskService{GetFn: func(_ context.Context, id int64) (domain.Task, error) {
			return taskOK(id), nil
		}}
		var inserted []domain.ValidateLog
		vd := &stubValidateLogRepository{BatchInsertFn: func(_ context.Context, logs []domain.ValidateLog) error {
			inserted = logs
			return nil
		}}
		src := &stubSource{Rows: []source.Row{
			{Table: "article", PK: "1", Cols: map[string]any{"title": "a"}},
			{Table: "article", PK: "2", Cols: map[string]any{"title": "b"}},
		}}
		dst := &stubSource{Rows: []source.Row{
			{Table: "article", PK: "1", Cols: map[string]any{"title": "a"}},
			{Table: "article", PK: "2", Cols: map[string]any{"title": "b"}},
		}}
		eng := newEngine(t, td, vd, src, dst, Config{})
		got, err := eng.Full(context.Background(), 1)
		require.NoError(t, err)
		assert.Equal(t, int64(0), got)
		assert.Empty(t, inserted)
	})

	t.Run("src 有 dst 无 → missing", func(t *testing.T) {
		td := &stubTaskService{GetFn: func(_ context.Context, id int64) (domain.Task, error) {
			return taskOK(id), nil
		}}
		var inserted []domain.ValidateLog
		vd := &stubValidateLogRepository{BatchInsertFn: func(_ context.Context, logs []domain.ValidateLog) error {
			inserted = logs
			return nil
		}}
		src := &stubSource{Rows: []source.Row{{PK: "1", Cols: map[string]any{"title": "a"}}}}
		dst := &stubSource{}
		eng := newEngine(t, td, vd, src, dst, Config{})
		got, err := eng.Full(context.Background(), 1)
		require.NoError(t, err)
		assert.Equal(t, int64(1), got)
		require.Len(t, inserted, 1)
		assert.Equal(t, "1", inserted[0].BizId)
		assert.Equal(t, consts.MismatchKindMissing, inserted[0].MismatchKind)
	})

	t.Run("dst 有 src 无 → extra", func(t *testing.T) {
		td := &stubTaskService{GetFn: func(_ context.Context, id int64) (domain.Task, error) {
			return taskOK(id), nil
		}}
		var inserted []domain.ValidateLog
		vd := &stubValidateLogRepository{BatchInsertFn: func(_ context.Context, logs []domain.ValidateLog) error {
			inserted = logs
			return nil
		}}
		src := &stubSource{}
		dst := &stubSource{Rows: []source.Row{{PK: "9", Cols: map[string]any{"title": "ghost"}}}}
		eng := newEngine(t, td, vd, src, dst, Config{})
		got, err := eng.Full(context.Background(), 1)
		require.NoError(t, err)
		assert.Equal(t, int64(1), got)
		require.Len(t, inserted, 1)
		assert.Equal(t, "9", inserted[0].BizId)
		assert.Equal(t, consts.MismatchKindExtra, inserted[0].MismatchKind)
	})

	t.Run("都有但字段不一致 → diff + diff_fields", func(t *testing.T) {
		td := &stubTaskService{GetFn: func(_ context.Context, id int64) (domain.Task, error) {
			return taskOK(id), nil
		}}
		var inserted []domain.ValidateLog
		vd := &stubValidateLogRepository{BatchInsertFn: func(_ context.Context, logs []domain.ValidateLog) error {
			inserted = logs
			return nil
		}}
		src := &stubSource{Rows: []source.Row{{PK: "1", Cols: map[string]any{"title": "a", "view": 100}}}}
		dst := &stubSource{Rows: []source.Row{{PK: "1", Cols: map[string]any{"title": "a", "view": 99}}}}
		eng := newEngine(t, td, vd, src, dst, Config{})
		got, err := eng.Full(context.Background(), 1)
		require.NoError(t, err)
		assert.Equal(t, int64(1), got)
		require.Len(t, inserted, 1)
		assert.Equal(t, consts.MismatchKindDiff, inserted[0].MismatchKind)
		assert.Contains(t, inserted[0].DiffDetail, "view")
		assert.Contains(t, inserted[0].DiffDetail, "diff_fields")
	})

	t.Run("混合差异 + 一致行：1 missing + 1 extra + 1 diff", func(t *testing.T) {
		td := &stubTaskService{GetFn: func(_ context.Context, id int64) (domain.Task, error) {
			return taskOK(id), nil
		}}
		var inserted []domain.ValidateLog
		vd := &stubValidateLogRepository{BatchInsertFn: func(_ context.Context, logs []domain.ValidateLog) error {
			inserted = logs
			return nil
		}}
		src := &stubSource{Rows: []source.Row{
			{PK: "1", Cols: map[string]any{"x": "a"}},  // 一致
			{PK: "2", Cols: map[string]any{"x": "b"}},  // missing
			{PK: "3", Cols: map[string]any{"x": "c1"}}, // diff
		}}
		dst := &stubSource{Rows: []source.Row{
			{PK: "1", Cols: map[string]any{"x": "a"}},
			{PK: "3", Cols: map[string]any{"x": "c2"}},
			{PK: "9", Cols: map[string]any{"x": "ghost"}}, // extra
		}}
		eng := newEngine(t, td, vd, src, dst, Config{})
		got, err := eng.Full(context.Background(), 1)
		require.NoError(t, err)
		assert.Equal(t, int64(3), got)
		assert.Len(t, inserted, 3)
	})
}

func TestVerifyEngine_Sample_InvalidRate(t *testing.T) {
	eng := newEngine(t, &stubTaskService{}, &stubValidateLogRepository{}, &stubSource{}, &stubSource{}, Config{})
	_, err := eng.Sample(context.Background(), 1, 0)
	assert.ErrorIs(t, err, ErrInvalidSampleRate)
	_, err = eng.Sample(context.Background(), 1, 1.5)
	assert.ErrorIs(t, err, ErrInvalidSampleRate)
}

func TestVerifyEngine_Repair(t *testing.T) {
	t.Run("MarkOnly 调 MarkRepaired + 返回 count", func(t *testing.T) {
		td := &stubTaskService{GetFn: func(_ context.Context, id int64) (domain.Task, error) {
			return taskOK(id), nil
		}}
		var capturedIDs []int64
		vd := &stubValidateLogRepository{MarkRepairedFn: func(_ context.Context, ids []int64) error {
			capturedIDs = ids
			return nil
		}}
		eng := newEngine(t, td, vd, &stubSource{}, &stubSource{}, Config{})
		got, err := eng.Repair(context.Background(), 1, RepairMarkOnly, []int64{10, 20, 30})
		require.NoError(t, err)
		assert.Equal(t, int64(3), got)
		assert.Equal(t, []int64{10, 20, 30}, capturedIDs)
	})

	t.Run("SrcOverwriteDst 完整路径：拿 snapshot → Sink.Apply → MarkRepaired", func(t *testing.T) {
		td := &stubTaskService{GetFn: func(_ context.Context, id int64) (domain.Task, error) {
			return taskOK(id), nil
		}}
		// validate_log 含两条 mismatch：一条 diff（src + dst 都有），一条 missing（src 有 dst 无）
		vd := &stubValidateLogRepository{
			FindByIDsFn: func(_ context.Context, ids []int64) ([]domain.ValidateLog, error) {
				return []domain.ValidateLog{
					{Id: 1, BizTable: "article", BizId: "100", MismatchKind: "diff",
						DiffDetail: `{"src":{"id":100,"title":"src-title"},"dst":{"id":100,"title":"dst-title"},"diff_fields":["title"]}`},
					{Id: 2, BizTable: "article", BizId: "200", MismatchKind: "missing",
						DiffDetail: `{"src":{"id":200,"title":"only-in-src"}}`},
				}, nil
			},
			MarkRepairedFn: func(_ context.Context, ids []int64) error {
				assert.ElementsMatch(t, []int64{1, 2}, ids)
				return nil
			},
		}
		var capturedBatch []sink.Mutation
		dstSink := &stubSinkForRepair{ApplyFn: func(_ context.Context, batch []sink.Mutation) error {
			capturedBatch = batch
			return nil
		}}
		eng := newEngineWithSinks(t, td, vd, &stubSource{}, &stubSource{}, nil, dstSink, Config{})

		count, err := eng.Repair(context.Background(), 1, RepairSrcOverwriteDst, []int64{1, 2})
		require.NoError(t, err)
		assert.Equal(t, int64(2), count)
		require.Len(t, capturedBatch, 2)
		// 第 1 条 diff → upsert src snapshot
		assert.Equal(t, sink.OpInsert, capturedBatch[0].Op)
		assert.Equal(t, "100", capturedBatch[0].PK)
		assert.Equal(t, "src-title", capturedBatch[0].Cols["title"])
		// 第 2 条 missing（src 有 dst 无）→ upsert src snapshot 到 dst
		assert.Equal(t, sink.OpInsert, capturedBatch[1].Op)
		assert.Equal(t, "200", capturedBatch[1].PK)
		assert.Equal(t, "only-in-src", capturedBatch[1].Cols["title"])
	})

	t.Run("SrcOverwriteDst extra 场景：src snapshot 缺 → delete dst biz_id", func(t *testing.T) {
		td := &stubTaskService{GetFn: func(_ context.Context, id int64) (domain.Task, error) {
			return taskOK(id), nil
		}}
		vd := &stubValidateLogRepository{
			FindByIDsFn: func(_ context.Context, _ []int64) ([]domain.ValidateLog, error) {
				return []domain.ValidateLog{
					{Id: 3, BizTable: "article", BizId: "99", MismatchKind: "extra",
						DiffDetail: `{"dst":{"id":99,"title":"ghost"}}`}, // src 缺
				}, nil
			},
		}
		var capturedBatch []sink.Mutation
		dstSink := &stubSinkForRepair{ApplyFn: func(_ context.Context, batch []sink.Mutation) error {
			capturedBatch = batch
			return nil
		}}
		eng := newEngineWithSinks(t, td, vd, &stubSource{}, &stubSource{}, nil, dstSink, Config{})

		_, err := eng.Repair(context.Background(), 1, RepairSrcOverwriteDst, []int64{3})
		require.NoError(t, err)
		require.Len(t, capturedBatch, 1)
		assert.Equal(t, sink.OpDelete, capturedBatch[0].Op)
		assert.Equal(t, "99", capturedBatch[0].PK)
	})

	t.Run("Repair 损坏 diff_detail → 跳过该行不抛错", func(t *testing.T) {
		td := &stubTaskService{GetFn: func(_ context.Context, id int64) (domain.Task, error) {
			return taskOK(id), nil
		}}
		vd := &stubValidateLogRepository{
			FindByIDsFn: func(_ context.Context, _ []int64) ([]domain.ValidateLog, error) {
				return []domain.ValidateLog{
					{Id: 4, BizTable: "article", BizId: "1", DiffDetail: "not-json"},
				}, nil
			},
		}
		var applyCalled bool
		dstSink := &stubSinkForRepair{ApplyFn: func(_ context.Context, _ []sink.Mutation) error {
			applyCalled = true
			return nil
		}}
		eng := newEngineWithSinks(t, td, vd, &stubSource{}, &stubSource{}, nil, dstSink, Config{})

		count, err := eng.Repair(context.Background(), 1, RepairSrcOverwriteDst, []int64{4})
		require.NoError(t, err)
		assert.Equal(t, int64(0), count, "损坏的 diff_detail 应被跳过")
		assert.False(t, applyCalled, "无有效 mutation 时不应调 Sink.Apply")
	})

	t.Run("未知 strategy → error", func(t *testing.T) {
		td := &stubTaskService{GetFn: func(_ context.Context, id int64) (domain.Task, error) {
			return taskOK(id), nil
		}}
		eng := newEngine(t, td, &stubValidateLogRepository{}, &stubSource{}, &stubSource{}, Config{})
		_, err := eng.Repair(context.Background(), 1, "foo", []int64{1})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unknown")
	})

	t.Run("task 不存在 → error", func(t *testing.T) {
		boom := errors.New("not found")
		td := &stubTaskService{GetFn: func(_ context.Context, _ int64) (domain.Task, error) {
			return domain.Task{}, boom
		}}
		eng := newEngine(t, td, &stubValidateLogRepository{}, &stubSource{}, &stubSource{}, Config{})
		_, err := eng.Repair(context.Background(), 1, RepairMarkOnly, []int64{1})
		assert.ErrorIs(t, err, boom)
	})
}

func TestDiffFields(t *testing.T) {
	t.Run("一致 → 空", func(t *testing.T) {
		got := diffFields(
			map[string]any{"a": 1, "b": "x"},
			map[string]any{"a": 1, "b": "x"},
		)
		assert.Empty(t, got)
	})
	t.Run("值不同 → 列出字段", func(t *testing.T) {
		got := diffFields(
			map[string]any{"a": 1, "b": "x"},
			map[string]any{"a": 1, "b": "y"},
		)
		assert.Equal(t, []string{"b"}, got)
	})
	t.Run("字段集不同 → 列出差", func(t *testing.T) {
		got := diffFields(
			map[string]any{"a": 1},
			map[string]any{"a": 1, "b": "y"},
		)
		assert.Equal(t, []string{"b"}, got)
	})
	t.Run("多字段差 → 字典序", func(t *testing.T) {
		got := diffFields(
			map[string]any{"z": 1, "a": 1, "m": 1},
			map[string]any{"z": 2, "a": 2, "m": 2},
		)
		assert.Equal(t, []string{"a", "m", "z"}, got)
	})
}
