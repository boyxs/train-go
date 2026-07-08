package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/boyxs/train-go/webook/migrator/domain"
	"github.com/boyxs/train-go/webook/migrator/errs"
	"github.com/boyxs/train-go/webook/migrator/repository/dao"
	"github.com/boyxs/train-go/webook/pkg/logger"
)

// ── stubTaskDAO 手写 stub，方法按调用闭包注入 ─────────────────
type stubTaskDAO struct {
	InsertFn            func(ctx context.Context, t dao.Task) (int64, error)
	FindByIdFn          func(ctx context.Context, id int64) (dao.Task, error)
	FindByNameFn        func(ctx context.Context, name string) (dao.Task, error)
	ListFn              func(ctx context.Context, status *int8, offset, limit int) ([]dao.Task, int64, error)
	UpdateStatusFn      func(ctx context.Context, id int64, status int8) error
	UpdateGrayPercentFn func(ctx context.Context, id int64, percent int16) error
	SoftDeleteFn        func(ctx context.Context, id int64) error
}

func (m *stubTaskDAO) Insert(ctx context.Context, t dao.Task) (int64, error) {
	return m.InsertFn(ctx, t)
}
func (m *stubTaskDAO) FindById(ctx context.Context, id int64) (dao.Task, error) {
	return m.FindByIdFn(ctx, id)
}
func (m *stubTaskDAO) FindByName(ctx context.Context, name string) (dao.Task, error) {
	return m.FindByNameFn(ctx, name)
}
func (m *stubTaskDAO) List(ctx context.Context, status *int8, offset, limit int) ([]dao.Task, int64, error) {
	return m.ListFn(ctx, status, offset, limit)
}
func (m *stubTaskDAO) UpdateStatus(ctx context.Context, id int64, status int8) error {
	return m.UpdateStatusFn(ctx, id, status)
}
func (m *stubTaskDAO) UpdateGrayPercent(ctx context.Context, id int64, percent int16) error {
	return m.UpdateGrayPercentFn(ctx, id, percent)
}
func (m *stubTaskDAO) SoftDelete(ctx context.Context, id int64) error {
	return m.SoftDeleteFn(ctx, id)
}

func TestInternalTaskRepository_Create(t *testing.T) {
	t.Run("成功", func(t *testing.T) {
		taskDAO := &stubTaskDAO{
			InsertFn: func(ctx context.Context, m dao.Task) (int64, error) {
				assert.Equal(t, "article_to_es_v1", m.Name)
				assert.Equal(t, "cdc", m.Mode)
				return 42, nil
			},
		}
		repo := NewTaskRepository(taskDAO, logger.NewNopLogger())

		id, err := repo.Create(context.Background(), domain.Task{
			Name: "article_to_es_v1", Mode: domain.ModeCDC, Kind: domain.KindHeterogeneous,
		})
		assert.NoError(t, err)
		assert.Equal(t, int64(42), id)
	})

	t.Run("DAO Insert 失败 → 错误透传", func(t *testing.T) {
		taskDAO := &stubTaskDAO{
			InsertFn: func(ctx context.Context, m dao.Task) (int64, error) {
				return 0, errs.ErrDuplicateTaskName
			},
		}
		repo := NewTaskRepository(taskDAO, logger.NewNopLogger())

		_, err := repo.Create(context.Background(), domain.Task{Name: "dup"})
		assert.ErrorIs(t, err, errs.ErrDuplicateTaskName)
	})
}

func TestInternalTaskRepository_FindById(t *testing.T) {
	t.Run("命中并映射回 domain", func(t *testing.T) {
		taskDAO := &stubTaskDAO{
			FindByIdFn: func(ctx context.Context, id int64) (dao.Task, error) {
				return dao.Task{
					Id: 7, Name: "user_nickname_v2", Mode: "dual_write", Kind: "schema",
					Status: 3, GrayPercent: 50, Consistency: "eventual",
					CreatedAt: 1000, UpdatedAt: 2000,
				}, nil
			},
		}
		repo := NewTaskRepository(taskDAO, logger.NewNopLogger())

		t1, err := repo.FindById(context.Background(), 7)
		assert.NoError(t, err)
		assert.Equal(t, int64(7), t1.Id)
		assert.Equal(t, domain.ModeDualWrite, t1.Mode)
		assert.Equal(t, domain.KindSchema, t1.Kind)
		assert.Equal(t, domain.TaskStatusIncrRunning, t1.Status)
		assert.Equal(t, int16(50), t1.GrayPercent)
	})

	t.Run("未找到 透传 ErrTaskNotFound", func(t *testing.T) {
		taskDAO := &stubTaskDAO{
			FindByIdFn: func(ctx context.Context, id int64) (dao.Task, error) {
				return dao.Task{}, errs.ErrTaskNotFound
			},
		}
		repo := NewTaskRepository(taskDAO, logger.NewNopLogger())

		_, err := repo.FindById(context.Background(), 999)
		assert.ErrorIs(t, err, errs.ErrTaskNotFound)
	})

	t.Run("其他错误透传", func(t *testing.T) {
		boom := errors.New("db down")
		taskDAO := &stubTaskDAO{
			FindByIdFn: func(ctx context.Context, id int64) (dao.Task, error) {
				return dao.Task{}, boom
			},
		}
		repo := NewTaskRepository(taskDAO, logger.NewNopLogger())

		_, err := repo.FindById(context.Background(), 1)
		assert.ErrorIs(t, err, boom)
	})
}

func TestInternalTaskRepository_List(t *testing.T) {
	t.Run("按 status 过滤 + 分页", func(t *testing.T) {
		taskDAO := &stubTaskDAO{
			ListFn: func(ctx context.Context, status *int8, offset, limit int) ([]dao.Task, int64, error) {
				assert.NotNil(t, status)
				assert.Equal(t, int8(3), *status)
				assert.Equal(t, 0, offset)
				assert.Equal(t, 10, limit)
				return []dao.Task{
					{Id: 2, Name: "t2", Mode: "cdc", Kind: "heterogeneous", Status: 3},
					{Id: 1, Name: "t1", Mode: "cdc", Kind: "heterogeneous", Status: 3},
				}, 2, nil
			},
		}
		repo := NewTaskRepository(taskDAO, logger.NewNopLogger())

		s := domain.TaskStatusIncrRunning
		list, total, err := repo.List(context.Background(), ListOpts{Status: &s, Offset: 0, Limit: 10})
		assert.NoError(t, err)
		assert.Equal(t, int64(2), total)
		assert.Len(t, list, 2)
		assert.Equal(t, int64(2), list[0].Id)
		assert.Equal(t, domain.ModeCDC, list[0].Mode)
	})

	t.Run("status 为 nil 时不过滤", func(t *testing.T) {
		taskDAO := &stubTaskDAO{
			ListFn: func(ctx context.Context, status *int8, offset, limit int) ([]dao.Task, int64, error) {
				assert.Nil(t, status)
				return []dao.Task{}, 0, nil
			},
		}
		repo := NewTaskRepository(taskDAO, logger.NewNopLogger())

		_, _, err := repo.List(context.Background(), ListOpts{Offset: 0, Limit: 50})
		assert.NoError(t, err)
	})
}
