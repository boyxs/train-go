package replay

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/boyxs/train-go/webook/migrator/domain"
	"github.com/boyxs/train-go/webook/migrator/pipeline/sink"
	"github.com/boyxs/train-go/webook/migrator/repository"
	"github.com/boyxs/train-go/webook/migrator/service"
	"github.com/boyxs/train-go/webook/pkg/logger"
)

// ── 手写 stub（接口嵌入 + 覆盖用到的方法）─────────────────
type stubTaskService struct {
	GetFn func(ctx context.Context, id int64) (domain.Task, error)
}

func (s *stubTaskService) Create(_ context.Context, _ service.CreateReq) (int64, error) {
	return 0, nil
}
func (s *stubTaskService) Get(ctx context.Context, id int64) (domain.Task, error) {
	if s.GetFn != nil {
		return s.GetFn(ctx, id)
	}
	return domain.Task{Id: id, TablesJSON: `[{"src":"article","dst":"article_v1","partitionKey":"id"}]`}, nil
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

type stubDeadLetterRepository struct {
	ListFn         func(ctx context.Context, taskId int64, limit int) ([]domain.DeadLetter, error)
	MarkReplayedFn func(ctx context.Context, ids []int64) error
	IncrementFn    func(ctx context.Context, id int64, lastErr string) error
}

func (s *stubDeadLetterRepository) ListUnreplayedByTask(ctx context.Context, taskId int64, limit int) ([]domain.DeadLetter, error) {
	if s.ListFn != nil {
		return s.ListFn(ctx, taskId, limit)
	}
	return nil, nil
}
func (s *stubDeadLetterRepository) MarkReplayed(ctx context.Context, ids []int64) error {
	if s.MarkReplayedFn != nil {
		return s.MarkReplayedFn(ctx, ids)
	}
	return nil
}
func (s *stubDeadLetterRepository) IncrementRetry(ctx context.Context, id int64, lastErr string) error {
	if s.IncrementFn != nil {
		return s.IncrementFn(ctx, id, lastErr)
	}
	return nil
}
func (s *stubDeadLetterRepository) CountUnreplayedByTask(_ context.Context) (map[int64]int64, error) {
	return nil, nil
}

type stubSink struct {
	ApplyFn func(ctx context.Context, batch []sink.Mutation) error
}

func (s *stubSink) Apply(ctx context.Context, batch []sink.Mutation) error {
	if s.ApplyFn != nil {
		return s.ApplyFn(ctx, batch)
	}
	return nil
}
func (s *stubSink) Close() error { return nil }

type stubSinkFactory struct {
	snk sink.Sink
}

func (f *stubSinkFactory) BuildSrc(_ context.Context, _ domain.Task, _ int) (sink.Sink, error) {
	return f.snk, nil
}
func (f *stubSinkFactory) BuildDst(_ context.Context, _ domain.Task, _ int) (sink.Sink, error) {
	return f.snk, nil
}

func newSvc(dl *stubDeadLetterRepository, snk sink.Sink) ReplayService {
	return NewReplayService(&stubTaskService{}, dl, &stubSinkFactory{snk: snk}, logger.NewNopLogger())
}

func TestReplayService_ReplayDeadLetters(t *testing.T) {
	t.Run("空 list → replayed=0 failed=0", func(t *testing.T) {
		svc := newSvc(&stubDeadLetterRepository{}, &stubSink{})
		replayed, failed, err := svc.ReplayDeadLetters(context.Background(), 1, 100)
		require.NoError(t, err)
		assert.Equal(t, int64(0), replayed)
		assert.Equal(t, int64(0), failed)
	})

	t.Run("3 条死信 → 全 replay 成功 + MarkReplayed", func(t *testing.T) {
		dl := &stubDeadLetterRepository{
			ListFn: func(_ context.Context, _ int64, _ int) ([]domain.DeadLetter, error) {
				return []domain.DeadLetter{
					{Id: 11, Op: "insert", BizTable: "article", BizId: "1", Payload: `{"id":1,"title":"a"}`},
					{Id: 12, Op: "update", BizTable: "article", BizId: "2", Payload: `{"id":2,"title":"b"}`},
					{Id: 13, Op: "delete", BizTable: "article", BizId: "3", Payload: `{"id":3}`},
				}, nil
			},
		}
		var markedIDs []int64
		dl.MarkReplayedFn = func(_ context.Context, ids []int64) error {
			markedIDs = ids
			return nil
		}
		var applyCount int
		snk := &stubSink{ApplyFn: func(_ context.Context, _ []sink.Mutation) error {
			applyCount++
			return nil
		}}
		replayed, failed, err := newSvc(dl, snk).ReplayDeadLetters(context.Background(), 1, 1000)
		require.NoError(t, err)
		assert.Equal(t, int64(3), replayed)
		assert.Equal(t, int64(0), failed)
		assert.Equal(t, 3, applyCount)
		assert.Equal(t, []int64{11, 12, 13}, markedIDs)
	})

	t.Run("payload 损坏 → IncrementRetry + failed 计数", func(t *testing.T) {
		dl := &stubDeadLetterRepository{
			ListFn: func(_ context.Context, _ int64, _ int) ([]domain.DeadLetter, error) {
				return []domain.DeadLetter{
					{Id: 21, Op: "insert", BizTable: "article", BizId: "1", Payload: "not-json"},
				}, nil
			},
		}
		var incrCalled int
		dl.IncrementFn = func(_ context.Context, _ int64, msg string) error {
			incrCalled++
			assert.Contains(t, msg, "payload unmarshal")
			return nil
		}
		replayed, failed, err := newSvc(dl, &stubSink{}).ReplayDeadLetters(context.Background(), 1, 1000)
		require.NoError(t, err)
		assert.Equal(t, int64(0), replayed)
		assert.Equal(t, int64(1), failed)
		assert.Equal(t, 1, incrCalled)
	})

	t.Run("Sink.Apply 失败 → IncrementRetry + failed 计数", func(t *testing.T) {
		dl := &stubDeadLetterRepository{
			ListFn: func(_ context.Context, _ int64, _ int) ([]domain.DeadLetter, error) {
				return []domain.DeadLetter{
					{Id: 31, Op: "insert", BizTable: "article", BizId: "1", Payload: `{"id":1}`},
				}, nil
			},
		}
		snk := &stubSink{ApplyFn: func(_ context.Context, _ []sink.Mutation) error {
			return errors.New("sink offline")
		}}
		var incrCalled int
		dl.IncrementFn = func(_ context.Context, _ int64, msg string) error {
			incrCalled++
			assert.Contains(t, msg, "sink offline")
			return nil
		}
		_, failed, err := newSvc(dl, snk).ReplayDeadLetters(context.Background(), 1, 1000)
		require.NoError(t, err)
		assert.Equal(t, int64(1), failed)
		assert.Equal(t, 1, incrCalled)
	})

	t.Run("biz_table 不在 task tables → failed 计数", func(t *testing.T) {
		dl := &stubDeadLetterRepository{
			ListFn: func(_ context.Context, _ int64, _ int) ([]domain.DeadLetter, error) {
				return []domain.DeadLetter{
					{Id: 41, Op: "insert", BizTable: "ghost_table", BizId: "1", Payload: `{"id":1}`},
				}, nil
			},
		}
		var incrCalled int
		dl.IncrementFn = func(_ context.Context, _ int64, msg string) error {
			incrCalled++
			assert.Contains(t, msg, "not in task tables")
			return nil
		}
		_, failed, err := newSvc(dl, &stubSink{}).ReplayDeadLetters(context.Background(), 1, 1000)
		require.NoError(t, err)
		assert.Equal(t, int64(1), failed)
		assert.Equal(t, 1, incrCalled)
	})
}
