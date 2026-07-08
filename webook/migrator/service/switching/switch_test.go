package switching

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/boyxs/train-go/webook/migrator/domain"
	migratorerrs "github.com/boyxs/train-go/webook/migrator/errs"
	"github.com/boyxs/train-go/webook/migrator/repository"
	"github.com/boyxs/train-go/webook/migrator/repository/cache"
	"github.com/boyxs/train-go/webook/migrator/repository/dao"
	"github.com/boyxs/train-go/webook/pkg/logger"
)

// ── stub TaskDAO ────────────────────────────────────────────
type stubTaskDAO struct {
	dao.TaskDAO
	FindByIdFn          func(ctx context.Context, id int64) (dao.Task, error)
	UpdateGrayPercentFn func(ctx context.Context, id int64, percent int16) error
}

func (s *stubTaskDAO) FindById(ctx context.Context, id int64) (dao.Task, error) {
	return s.FindByIdFn(ctx, id)
}
func (s *stubTaskDAO) UpdateGrayPercent(ctx context.Context, id int64, percent int16) error {
	if s.UpdateGrayPercentFn != nil {
		return s.UpdateGrayPercentFn(ctx, id, percent)
	}
	return nil
}
func (s *stubTaskDAO) UpdateStatus(_ context.Context, _ int64, _ int8) error {
	// 测试中默认无操作；如需断言 status 推进，再用具体 mock 覆盖
	return nil
}

func newSvc(t *testing.T, td dao.TaskDAO) (SwitchService, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	cli := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	repo := repository.NewTaskRepository(td, logger.NewNopLogger())
	state := repository.NewSwitchStateRepository(cache.NewRedisSwitchStateCache(cli))
	return NewSwitchService(repo, state, logger.NewNopLogger()), mr
}

func taskOK(id int64) dao.Task {
	return dao.Task{Id: id, Name: "task_" + strconv.FormatInt(id, 10)}
}

// ── SetGray / GetGray ───────────────────────────────────────

func TestSwitchService_Gray(t *testing.T) {
	t.Run("set + get 来回", func(t *testing.T) {
		td := &stubTaskDAO{FindByIdFn: func(_ context.Context, id int64) (dao.Task, error) {
			return taskOK(id), nil
		}}
		svc, _ := newSvc(t, td)
		ctx := context.Background()
		require.NoError(t, svc.SetGray(ctx, 1, 50))
		got, err := svc.GetGray(ctx, 1)
		require.NoError(t, err)
		assert.Equal(t, 50, got)
	})

	t.Run("默认 0", func(t *testing.T) {
		svc, _ := newSvc(t, &stubTaskDAO{FindByIdFn: func(_ context.Context, id int64) (dao.Task, error) {
			return taskOK(id), nil
		}})
		got, err := svc.GetGray(context.Background(), 999)
		require.NoError(t, err)
		assert.Equal(t, 0, got)
	})

	t.Run("超界 → ErrInvalidGrayPercent", func(t *testing.T) {
		svc, _ := newSvc(t, &stubTaskDAO{})
		assert.ErrorIs(t, svc.SetGray(context.Background(), 1, -1), migratorerrs.ErrInvalidGrayPercent)
		assert.ErrorIs(t, svc.SetGray(context.Background(), 1, 101), migratorerrs.ErrInvalidGrayPercent)
	})

	t.Run("同步 task.gray_percent", func(t *testing.T) {
		var capturedID int64
		var capturedPct int16
		td := &stubTaskDAO{
			FindByIdFn: func(_ context.Context, id int64) (dao.Task, error) { return taskOK(id), nil },
			UpdateGrayPercentFn: func(_ context.Context, id int64, p int16) error {
				capturedID = id
				capturedPct = p
				return nil
			},
		}
		svc, _ := newSvc(t, td)
		require.NoError(t, svc.SetGray(context.Background(), 7, 25))
		assert.Equal(t, int64(7), capturedID)
		assert.Equal(t, int16(25), capturedPct)
	})

	t.Run("task 不存在 → error", func(t *testing.T) {
		boom := errors.New("not found")
		svc, _ := newSvc(t, &stubTaskDAO{FindByIdFn: func(_ context.Context, _ int64) (dao.Task, error) {
			return dao.Task{}, boom
		}})
		assert.ErrorIs(t, svc.SetGray(context.Background(), 1, 50), boom)
	})
}

// ── SetStage / GetStage / 状态机 ───────────────────────────

func TestSwitchService_SetStage(t *testing.T) {
	td := &stubTaskDAO{FindByIdFn: func(_ context.Context, id int64) (dao.Task, error) {
		return taskOK(id), nil
	}}

	t.Run("默认 SRC_ONLY → SRC_FIRST 合法", func(t *testing.T) {
		svc, _ := newSvc(t, td)
		require.NoError(t, svc.SetStage(context.Background(), 1, domain.StageSrcFirst, "", ""))
		got, err := svc.GetStage(context.Background(), 1)
		require.NoError(t, err)
		assert.Equal(t, domain.StageSrcFirst, got)
	})

	t.Run("跳级 SRC_ONLY → DST_FIRST 非法", func(t *testing.T) {
		svc, _ := newSvc(t, td)
		err := svc.SetStage(context.Background(), 1, domain.StageDstFirst, "", "")
		assert.ErrorIs(t, err, migratorerrs.ErrInvalidStatusTransition)
	})

	t.Run("幂等：同 stage 重复 SetStage 不报错", func(t *testing.T) {
		svc, _ := newSvc(t, td)
		require.NoError(t, svc.SetStage(context.Background(), 1, domain.StageSrcFirst, "", ""))
		require.NoError(t, svc.SetStage(context.Background(), 1, domain.StageSrcFirst, "", ""))
	})

	t.Run("非法 stage 值", func(t *testing.T) {
		svc, _ := newSvc(t, td)
		err := svc.SetStage(context.Background(), 1, "foobar", "", "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid stage")
	})

	t.Run("完整链路 SRC_ONLY → SRC_FIRST → DST_FIRST → DST_ONLY（含双人复核）", func(t *testing.T) {
		svc, _ := newSvc(t, td)
		ctx := context.Background()
		require.NoError(t, svc.SetStage(ctx, 1, domain.StageSrcFirst, "", ""))
		require.NoError(t, svc.SetStage(ctx, 1, domain.StageDstFirst, "", ""))

		// 直接 propose + approve（双人不同）→ 通过
		require.NoError(t, svc.SetStage(ctx, 1, domain.StageDstOnly, "alice", "bob"))
		got, err := svc.GetStage(ctx, 1)
		require.NoError(t, err)
		assert.Equal(t, domain.StageDstOnly, got)
	})
}

// ── 双人复核 ──────────────────────────────────────────────

func TestSwitchService_DualApproval(t *testing.T) {
	td := &stubTaskDAO{FindByIdFn: func(_ context.Context, id int64) (dao.Task, error) {
		return taskOK(id), nil
	}}

	t.Run("只 propose 没 approve → ErrProposeNotFound，但 Redis 已注册 propose", func(t *testing.T) {
		svc, mr := newSvc(t, td)
		ctx := context.Background()
		require.NoError(t, svc.SetStage(ctx, 7, domain.StageSrcFirst, "", ""))
		require.NoError(t, svc.SetStage(ctx, 7, domain.StageDstFirst, "", ""))

		err := svc.SetStage(ctx, 7, domain.StageDstOnly, "alice", "")
		assert.ErrorIs(t, err, ErrProposeNotFound)
		// Redis 中应有 propose
		assert.True(t, mr.Exists(cache.KeyCutoverPropose+"task_7"))
		v, _ := mr.Get(cache.KeyCutoverPropose + "task_7")
		assert.Equal(t, "alice", v)
	})

	t.Run("第二次 approve 必须不同 actor", func(t *testing.T) {
		svc, _ := newSvc(t, td)
		ctx := context.Background()
		require.NoError(t, svc.SetStage(ctx, 8, domain.StageSrcFirst, "", ""))
		require.NoError(t, svc.SetStage(ctx, 8, domain.StageDstFirst, "", ""))

		// 第一步 propose
		_ = svc.SetStage(ctx, 8, domain.StageDstOnly, "alice", "")
		// 第二步 approve - 同 actor 失败
		err := svc.SetStage(ctx, 8, domain.StageDstOnly, "", "alice")
		assert.ErrorIs(t, err, ErrApprovalSameActor)
	})

	t.Run("第二步 approve - 不同 actor 通过 + 删 propose key", func(t *testing.T) {
		svc, mr := newSvc(t, td)
		ctx := context.Background()
		require.NoError(t, svc.SetStage(ctx, 9, domain.StageSrcFirst, "", ""))
		require.NoError(t, svc.SetStage(ctx, 9, domain.StageDstFirst, "", ""))

		_ = svc.SetStage(ctx, 9, domain.StageDstOnly, "alice", "")
		require.True(t, mr.Exists(cache.KeyCutoverPropose+"task_9"))

		require.NoError(t, svc.SetStage(ctx, 9, domain.StageDstOnly, "", "bob"))
		// propose key 应已删
		assert.False(t, mr.Exists(cache.KeyCutoverPropose+"task_9"))
		got, _ := svc.GetStage(ctx, 9)
		assert.Equal(t, domain.StageDstOnly, got)
	})

	t.Run("没 propose 直接 approve → ErrProposeNotFound", func(t *testing.T) {
		svc, _ := newSvc(t, td)
		ctx := context.Background()
		require.NoError(t, svc.SetStage(ctx, 10, domain.StageSrcFirst, "", ""))
		require.NoError(t, svc.SetStage(ctx, 10, domain.StageDstFirst, "", ""))

		err := svc.SetStage(ctx, 10, domain.StageDstOnly, "", "bob")
		assert.ErrorIs(t, err, ErrProposeNotFound)
	})

	t.Run("propose + approve 同次提供不同 actor → 直接通过", func(t *testing.T) {
		svc, _ := newSvc(t, td)
		ctx := context.Background()
		require.NoError(t, svc.SetStage(ctx, 11, domain.StageSrcFirst, "", ""))
		require.NoError(t, svc.SetStage(ctx, 11, domain.StageDstFirst, "", ""))
		require.NoError(t, svc.SetStage(ctx, 11, domain.StageDstOnly, "alice", "bob"))
	})
}

// ── Rollback ────────────────────────────────────────────────

func TestSwitchService_Rollback(t *testing.T) {
	td := &stubTaskDAO{FindByIdFn: func(_ context.Context, id int64) (dao.Task, error) {
		return taskOK(id), nil
	}}

	t.Run("DST_FIRST → rollback 到 SRC_FIRST", func(t *testing.T) {
		svc, mr := newSvc(t, td)
		ctx := context.Background()
		require.NoError(t, svc.SetStage(ctx, 1, domain.StageSrcFirst, "", ""))
		require.NoError(t, svc.SetStage(ctx, 1, domain.StageDstFirst, "", ""))
		require.NoError(t, svc.Rollback(ctx, 1))
		v, _ := mr.Get(cache.KeyStage + "task_1")
		assert.Equal(t, string(domain.StageSrcFirst), v)
	})

	t.Run("DST_ONLY → rollback 被拒（单写不可逆）", func(t *testing.T) {
		svc, _ := newSvc(t, td)
		ctx := context.Background()
		require.NoError(t, svc.SetStage(ctx, 1, domain.StageSrcFirst, "", ""))
		require.NoError(t, svc.SetStage(ctx, 1, domain.StageDstFirst, "", ""))
		require.NoError(t, svc.SetStage(ctx, 1, domain.StageDstOnly, "a", "b"))
		assert.ErrorIs(t, svc.Rollback(ctx, 1), migratorerrs.ErrRollbackNotAllowed)
		got, _ := svc.GetStage(ctx, 1)
		assert.Equal(t, domain.StageDstOnly, got) // 拒绝后 stage 保持不变
	})

	t.Run("SRC_ONLY → rollback 被拒（未进双写期）", func(t *testing.T) {
		svc, _ := newSvc(t, td)
		// 默认 SRC_ONLY（未 SetStage）
		assert.ErrorIs(t, svc.Rollback(context.Background(), 1), migratorerrs.ErrRollbackNotAllowed)
	})

	t.Run("rollback 幂等：已在 SRC_FIRST → no-op", func(t *testing.T) {
		svc, _ := newSvc(t, td)
		ctx := context.Background()
		require.NoError(t, svc.SetStage(ctx, 1, domain.StageSrcFirst, "", ""))
		require.NoError(t, svc.Rollback(ctx, 1))
		got, _ := svc.GetStage(ctx, 1)
		assert.Equal(t, domain.StageSrcFirst, got)
	})

	t.Run("task 不存在 → error", func(t *testing.T) {
		boom := errors.New("not found")
		svc, _ := newSvc(t, &stubTaskDAO{FindByIdFn: func(_ context.Context, _ int64) (dao.Task, error) {
			return dao.Task{}, boom
		}})
		assert.ErrorIs(t, svc.Rollback(context.Background(), 1), boom)
	})
}

func TestStage_Valid(t *testing.T) {
	for _, s := range []domain.Stage{domain.StageSrcOnly, domain.StageSrcFirst, domain.StageDstFirst, domain.StageDstOnly} {
		assert.True(t, s.Valid(), s)
	}
	assert.False(t, domain.Stage("foobar").Valid())
	assert.False(t, domain.Stage("").Valid())
}

func TestStage_CanTransitionTo(t *testing.T) {
	testCases := []struct {
		from, to domain.Stage
		ok       bool
	}{
		{domain.StageSrcOnly, domain.StageSrcFirst, true},
		{domain.StageSrcFirst, domain.StageDstFirst, true},
		{domain.StageDstFirst, domain.StageDstOnly, true},
		{domain.StageSrcOnly, domain.StageDstFirst, false}, // 跳级
		{domain.StageSrcOnly, domain.StageDstOnly, false},  // 跳级
		{domain.StageDstOnly, domain.StageSrcOnly, false},  // 倒退（rollback 走独立方法）
		{domain.StageSrcFirst, domain.StageSrcOnly, false}, // 倒退
	}
	for _, c := range testCases {
		assert.Equal(t, c.ok, c.from.CanTransitionTo(c.to),
			strconv.Itoa(int(c.from[0]))+"→"+strconv.Itoa(int(c.to[0])))
	}
}

func TestStage_CanRollback(t *testing.T) {
	assert.True(t, domain.StageSrcFirst.CanRollback())
	assert.True(t, domain.StageDstFirst.CanRollback())
	assert.False(t, domain.StageSrcOnly.CanRollback()) // 未进双写
	assert.False(t, domain.StageDstOnly.CanRollback()) // 单写不可逆
}
