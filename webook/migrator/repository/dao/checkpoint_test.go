package dao

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"

	"github.com/boyxs/train-go/webook/migrator/consts"
	"github.com/boyxs/train-go/webook/migrator/errs"
)

func TestGormCheckpointDAO_Upsert(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	assert.NoError(t, err)
	mock.ExpectExec("INSERT INTO `checkpoint`").
		WillReturnResult(sqlmock.NewResult(1, 1))

	dao := NewGormCheckpointDAO(openMockGorm(t, sqlDB))
	gotId, err := dao.Upsert(context.Background(), Checkpoint{
		TaskId:     1,
		Phase:      consts.PhaseFull,
		ShardNo:    0,
		CursorKind: consts.CursorKindIDRange,
	})
	assert.NoError(t, err)
	assert.Equal(t, int64(1), gotId)
}

func TestGormCheckpointDAO_FindByTaskAndPhase(t *testing.T) {
	t.Run("多分片返回", func(t *testing.T) {
		sqlDB, mock, err := sqlmock.New()
		assert.NoError(t, err)
		rows := sqlmock.NewRows([]string{
			"id", "task_id", "phase", "shard_no", "cursor_kind", "cursor_value",
			"progress_percent", "last_lag_ms", "version", "updated_at",
		}).
			AddRow(1, 100, "full", 0, "id_range", `{"min":1,"max":1000}`, 50.0, int64(0), int64(1), int64(2000)).
			AddRow(2, 100, "full", 1, "id_range", `{"min":1001,"max":2000}`, 80.0, int64(0), int64(2), int64(3000))
		mock.ExpectQuery("SELECT \\* FROM `checkpoint`").WillReturnRows(rows)

		dao := NewGormCheckpointDAO(openMockGorm(t, sqlDB))
		list, err := dao.FindByTaskAndPhase(context.Background(), 100, "full")
		assert.NoError(t, err)
		assert.Len(t, list, 2)
		assert.Equal(t, int32(0), list[0].ShardNo)
		assert.Equal(t, int32(1), list[1].ShardNo)
		assert.Equal(t, int64(1), list[0].Version)
	})

	t.Run("空结果", func(t *testing.T) {
		sqlDB, mock, err := sqlmock.New()
		assert.NoError(t, err)
		mock.ExpectQuery("SELECT \\* FROM `checkpoint`").
			WillReturnRows(sqlmock.NewRows([]string{"id"}))

		dao := NewGormCheckpointDAO(openMockGorm(t, sqlDB))
		list, err := dao.FindByTaskAndPhase(context.Background(), 999, "incr")
		assert.NoError(t, err)
		assert.Empty(t, list)
	})
}

func TestGormCheckpointDAO_UpdateCursor(t *testing.T) {
	t.Run("乐观锁成功 (version 匹配)", func(t *testing.T) {
		sqlDB, mock, err := sqlmock.New()
		assert.NoError(t, err)
		// rows_affected = 1 表示 WHERE version = expectedVersion 命中
		mock.ExpectExec("UPDATE `checkpoint`").
			WillReturnResult(sqlmock.NewResult(0, 1))

		dao := NewGormCheckpointDAO(openMockGorm(t, sqlDB))
		err = dao.UpdateCursor(context.Background(), CheckpointUpdate{
			TaskId:          100,
			Phase:           consts.PhaseFull,
			ShardNo:         0,
			CursorValue:     `{"min":1,"max":2000}`,
			ProgressPercent: 80.0,
			LagMs:           100,
			ExpectedVersion: 1,
		})
		assert.NoError(t, err)
	})

	t.Run("乐观锁冲突 (version 不匹配 → rows=0)", func(t *testing.T) {
		sqlDB, mock, err := sqlmock.New()
		assert.NoError(t, err)
		// rows_affected = 0 表示其他 worker 已更新过 version
		mock.ExpectExec("UPDATE `checkpoint`").
			WillReturnResult(sqlmock.NewResult(0, 0))

		dao := NewGormCheckpointDAO(openMockGorm(t, sqlDB))
		err = dao.UpdateCursor(context.Background(), CheckpointUpdate{
			TaskId:          100,
			Phase:           consts.PhaseFull,
			ShardNo:         0,
			CursorValue:     `{"min":1,"max":2000}`,
			ProgressPercent: 80.0,
			LagMs:           100,
			ExpectedVersion: 1, // 已过期
		})
		assert.ErrorIs(t, err, errs.ErrCheckpointVersionConflict)
	})
}

func TestGormCheckpointDAO_DeleteByTask(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	assert.NoError(t, err)
	mock.ExpectExec("DELETE FROM `checkpoint`").
		WillReturnResult(sqlmock.NewResult(0, 16))

	dao := NewGormCheckpointDAO(openMockGorm(t, sqlDB))
	err = dao.DeleteByTask(context.Background(), 100)
	assert.NoError(t, err)
}
