package dao

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"

	"github.com/webook/migrator/consts"
)

// ── validate_log ───────────────────────────────

func TestGormValidateLogDAO_Insert(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	assert.NoError(t, err)
	mock.ExpectExec("INSERT INTO `validate_log`").
		WillReturnResult(sqlmock.NewResult(1, 1))

	dao := NewGormValidateLogDAO(openMockGorm(t, sqlDB))
	id, err := dao.Insert(context.Background(), ValidateLog{
		TaskId:       100,
		Direction:    consts.DirectionSrcToDst,
		BizTable:     "article",
		BizId:        "99887",
		MismatchKind: consts.MismatchKindDiff,
		DiffDetail:   `{"src.title":"foo","dst.title":"bar"}`,
	})
	assert.NoError(t, err)
	assert.Equal(t, int64(1), id)
}

func TestGormValidateLogDAO_BatchInsert(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	assert.NoError(t, err)
	mock.ExpectExec("INSERT INTO `validate_log`").
		WillReturnResult(sqlmock.NewResult(0, 2))

	dao := NewGormValidateLogDAO(openMockGorm(t, sqlDB))
	err = dao.BatchInsert(context.Background(), []ValidateLog{
		{TaskId: 100, Direction: consts.DirectionSrcToDst, BizTable: "article", BizId: "1", MismatchKind: consts.MismatchKindMissing},
		{TaskId: 100, Direction: consts.DirectionSrcToDst, BizTable: "article", BizId: "2", MismatchKind: consts.MismatchKindMissing},
	})
	assert.NoError(t, err)
}

func TestGormValidateLogDAO_BatchInsert_Empty(t *testing.T) {
	sqlDB, _, err := sqlmock.New()
	assert.NoError(t, err)
	dao := NewGormValidateLogDAO(openMockGorm(t, sqlDB))
	// 空切片不发 SQL
	err = dao.BatchInsert(context.Background(), nil)
	assert.NoError(t, err)
}

func TestGormValidateLogDAO_ListUnrepaired(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	assert.NoError(t, err)
	mock.ExpectQuery("SELECT count").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	rows := sqlmock.NewRows([]string{
		"id", "task_id", "direction", "table_name", "biz_id", "mismatch_kind",
		"diff_detail", "repaired", "created_at", "repaired_at",
	}).
		AddRow(1, 100, "src_to_dst", "article", "99887", "diff", "{}", 0, int64(1000), int64(0)).
		AddRow(2, 100, "src_to_dst", "article", "99892", "missing", "", 0, int64(2000), int64(0))
	mock.ExpectQuery("SELECT \\* FROM `validate_log`").WillReturnRows(rows)

	dao := NewGormValidateLogDAO(openMockGorm(t, sqlDB))
	list, total, err := dao.ListUnrepaired(context.Background(), 100, 0, 50)
	assert.NoError(t, err)
	assert.Equal(t, int64(2), total)
	assert.Len(t, list, 2)
}

func TestGormValidateLogDAO_MarkRepaired(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	assert.NoError(t, err)
	mock.ExpectExec("UPDATE `validate_log`").
		WillReturnResult(sqlmock.NewResult(0, 3))

	dao := NewGormValidateLogDAO(openMockGorm(t, sqlDB))
	err = dao.MarkRepaired(context.Background(), []int64{1, 2, 3})
	assert.NoError(t, err)
}

// ── audit_log ──────────────────────────────────

func TestGormAuditLogDAO_Insert(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	assert.NoError(t, err)
	mock.ExpectExec("INSERT INTO `audit_log`").
		WillReturnResult(sqlmock.NewResult(1, 1))

	dao := NewGormAuditLogDAO(openMockGorm(t, sqlDB))
	id, err := dao.Insert(context.Background(), AuditLog{
		TaskId:   100,
		Actor:    "admin-A",
		Action:   consts.AuditActionCutoverPropose,
		Payload:  `{"stage":"DST_ONLY","action":"propose"}`,
		Result:   consts.AuditResultSuccess,
		ClientIp: "10.0.0.1",
	})
	assert.NoError(t, err)
	assert.Equal(t, int64(1), id)
}

// ── dead_letter ──────────────────────────────────────────

func TestGormDeadLetterDAO_Insert(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	assert.NoError(t, err)
	mock.ExpectExec("INSERT INTO `dead_letter`").
		WillReturnResult(sqlmock.NewResult(1, 1))

	dao := NewGormDeadLetterDAO(openMockGorm(t, sqlDB))
	id, err := dao.Insert(context.Background(), DeadLetter{
		TaskId:     100,
		Op:         consts.DLOpUpdate,
		BizTable:   "user",
		BizId:      "42",
		Payload:    `{"id":42,"nickname_v2":"new"}`,
		LastError:  "connection refused",
		RetryCount: 3,
	})
	assert.NoError(t, err)
	assert.Equal(t, int64(1), id)
}

func TestGormDeadLetterDAO_ListUnreplayedByTask(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	assert.NoError(t, err)
	rows := sqlmock.NewRows([]string{
		"id", "task_id", "op", "table_name", "biz_id", "payload",
		"last_error", "retry_count", "replayed", "replay_failed", "created_at", "replayed_at",
	}).
		AddRow(1, 100, "update", "user", "42", "{}", "err1", 3, 0, 0, int64(1000), int64(0)).
		AddRow(2, 100, "insert", "user", "43", "{}", "err2", 3, 0, 0, int64(2000), int64(0))
	mock.ExpectQuery("SELECT \\* FROM `dead_letter`").WillReturnRows(rows)

	dao := NewGormDeadLetterDAO(openMockGorm(t, sqlDB))
	list, err := dao.ListUnreplayedByTask(context.Background(), 100, 50)
	assert.NoError(t, err)
	assert.Len(t, list, 2)
}

func TestGormDeadLetterDAO_IncrementRetry(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	assert.NoError(t, err)
	mock.ExpectExec("UPDATE `dead_letter`").
		WillReturnResult(sqlmock.NewResult(0, 1))

	dao := NewGormDeadLetterDAO(openMockGorm(t, sqlDB))
	err = dao.IncrementRetry(context.Background(), 1, "next error")
	assert.NoError(t, err)
}

func TestGormDeadLetterDAO_MarkReplayed(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	assert.NoError(t, err)
	mock.ExpectExec("UPDATE `dead_letter`").
		WillReturnResult(sqlmock.NewResult(0, 2))

	dao := NewGormDeadLetterDAO(openMockGorm(t, sqlDB))
	err = dao.MarkReplayed(context.Background(), []int64{1, 2})
	assert.NoError(t, err)
}

func TestGormDeadLetterDAO_MarkReplayFailed(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	assert.NoError(t, err)
	mock.ExpectExec("UPDATE `dead_letter`").
		WillReturnResult(sqlmock.NewResult(0, 1))

	dao := NewGormDeadLetterDAO(openMockGorm(t, sqlDB))
	err = dao.MarkReplayFailed(context.Background(), []int64{99}, "exhausted retries")
	assert.NoError(t, err)
}
