package sink

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"github.com/webook/pkg/logger"
)

func newSink(t *testing.T) (Sink, sqlmock.Sqlmock, func()) {
	t.Helper()
	mockDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	gormDB, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      mockDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	require.NoError(t, err)
	sink := NewMySQLSink(gormDB, "article", "id", logger.NewNopLogger())
	return sink, mock, func() { _ = mockDB.Close() }
}

func TestMySQLSink_Apply(t *testing.T) {
	t.Run("空 batch 不发任何 SQL", func(t *testing.T) {
		sink, mock, cleanup := newSink(t)
		defer cleanup()

		err := sink.Apply(context.Background(), nil)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("纯 upsert：insert + update 合并到一条 ON DUPLICATE KEY UPDATE", func(t *testing.T) {
		sink, mock, cleanup := newSink(t)
		defer cleanup()

		mock.ExpectBegin()
		mock.ExpectExec(
			"INSERT INTO `article` (`id`,`title`) VALUES (?,?),(?,?) ON DUPLICATE KEY UPDATE `id`=VALUES(`id`),`title`=VALUES(`title`)",
		).WithArgs(int64(1), "a", int64(2), "b").WillReturnResult(sqlmock.NewResult(0, 2))
		mock.ExpectCommit()

		err := sink.Apply(context.Background(), []Mutation{
			{Op: OpInsert, Table: "article", PK: "1", Cols: map[string]any{"id": int64(1), "title": "a"}},
			{Op: OpUpdate, Table: "article", PK: "2", Cols: map[string]any{"id": int64(2), "title": "b"}},
		})
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("纯 delete：单 DELETE IN", func(t *testing.T) {
		sink, mock, cleanup := newSink(t)
		defer cleanup()

		mock.ExpectBegin()
		mock.ExpectExec("DELETE FROM `article` WHERE `id` IN (?,?)").
			WithArgs("7", "8").
			WillReturnResult(sqlmock.NewResult(0, 2))
		mock.ExpectCommit()

		err := sink.Apply(context.Background(), []Mutation{
			{Op: OpDelete, Table: "article", PK: "7"},
			{Op: OpDelete, Table: "article", PK: "8"},
		})
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("混合：先 DELETE 后 UPSERT，同事务", func(t *testing.T) {
		sink, mock, cleanup := newSink(t)
		defer cleanup()

		mock.ExpectBegin()
		mock.ExpectExec("DELETE FROM `article` WHERE `id` IN (?)").
			WithArgs("9").
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(
			"INSERT INTO `article` (`id`,`title`) VALUES (?,?) ON DUPLICATE KEY UPDATE `id`=VALUES(`id`),`title`=VALUES(`title`)",
		).WithArgs(int64(1), "x").WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		err := sink.Apply(context.Background(), []Mutation{
			{Op: OpDelete, Table: "article", PK: "9"},
			{Op: OpUpdate, Table: "article", PK: "1", Cols: map[string]any{"id": int64(1), "title": "x"}},
		})
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("不支持的 op 返回 error", func(t *testing.T) {
		sink, _, cleanup := newSink(t)
		defer cleanup()

		err := sink.Apply(context.Background(), []Mutation{
			{Op: "patch", PK: "1", Cols: map[string]any{"id": int64(1)}},
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported op")
	})

	t.Run("upsert 但 Cols 为 nil 返回 error", func(t *testing.T) {
		sink, _, cleanup := newSink(t)
		defer cleanup()

		err := sink.Apply(context.Background(), []Mutation{
			{Op: OpInsert, PK: "1", Cols: nil},
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "nil Cols")
	})

	t.Run("delete SQL 失败 → 事务回滚 + 错误向上传播", func(t *testing.T) {
		sink, mock, cleanup := newSink(t)
		defer cleanup()

		boom := errors.New("disk full")
		mock.ExpectBegin()
		mock.ExpectExec("DELETE FROM `article` WHERE `id` IN (?)").
			WithArgs("9").
			WillReturnError(boom)
		mock.ExpectRollback()

		err := sink.Apply(context.Background(), []Mutation{{Op: OpDelete, PK: "9"}})
		assert.ErrorIs(t, err, boom)
	})
}

// ── Version 乐观锁路径（防老 binlog 覆盖新值） ───────────

func TestMySQLSink_Apply_OptimisticLock(t *testing.T) {
	t.Run("upsert 含 version 列 → SQL 启用乐观锁 IF/GREATEST", func(t *testing.T) {
		sink, mock, cleanup := newSink(t)
		defer cleanup()

		mock.ExpectBegin()
		mock.ExpectExec(
			"INSERT INTO `article` (`id`,`title`,`version`) VALUES (?,?,?) "+
				"ON DUPLICATE KEY UPDATE "+
				"`id`=IF(VALUES(`version`) > `version`, VALUES(`id`), `id`),"+
				"`title`=IF(VALUES(`version`) > `version`, VALUES(`title`), `title`),"+
				"`version`=GREATEST(`version`, VALUES(`version`))",
		).WithArgs(int64(1), "a", int64(100)).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		err := sink.Apply(context.Background(), []Mutation{
			{Op: OpUpdate, Table: "article", PK: "1", Cols: map[string]any{
				"id": int64(1), "title": "a", "version": int64(100),
			}},
		})
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("batch 部分行无 version → 不启用乐观锁", func(t *testing.T) {
		sink, mock, cleanup := newSink(t)
		defer cleanup()

		// 一行有 version 一行没有 → 整批走原 SET = VALUES 路径
		mock.ExpectBegin()
		mock.ExpectExec(
			"INSERT INTO `article` (`id`,`title`,`version`) VALUES (?,?,?),(?,?,?) "+
				"ON DUPLICATE KEY UPDATE `id`=VALUES(`id`),`title`=VALUES(`title`),`version`=VALUES(`version`)",
		).WithArgs(int64(1), "a", int64(100), int64(2), "b", nil).
			WillReturnResult(sqlmock.NewResult(0, 2))
		mock.ExpectCommit()

		err := sink.Apply(context.Background(), []Mutation{
			{Op: OpInsert, Table: "article", PK: "1", Cols: map[string]any{
				"id": int64(1), "title": "a", "version": int64(100),
			}},
			{Op: OpInsert, Table: "article", PK: "2", Cols: map[string]any{
				"id": int64(2), "title": "b",
			}},
		})
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestAllRowsHaveColumn(t *testing.T) {
	t.Run("全有 → true", func(t *testing.T) {
		got := allRowsHaveColumn([]map[string]any{
			{"id": 1, "version": 100},
			{"id": 2, "version": 50},
		}, "version")
		assert.True(t, got)
	})
	t.Run("有一行缺 → false", func(t *testing.T) {
		got := allRowsHaveColumn([]map[string]any{
			{"id": 1, "version": 100},
			{"id": 2}, // 缺 version
		}, "version")
		assert.False(t, got)
	})
	t.Run("空 rows → false", func(t *testing.T) {
		got := allRowsHaveColumn(nil, "version")
		assert.False(t, got)
	})
}

func TestBuildAssignments(t *testing.T) {
	t.Run("optLock=false → SET = VALUES", func(t *testing.T) {
		got := buildAssignments([]string{"id", "title"}, false)
		assert.Len(t, got, 2)
		assert.Contains(t, got, "id")
		assert.Contains(t, got, "title")
	})
	t.Run("optLock=true → SET IF(VALUES(version)>version,...) + GREATEST", func(t *testing.T) {
		got := buildAssignments([]string{"id", "title", "version"}, true)
		assert.Len(t, got, 3)
		assert.Contains(t, got, "id")
		assert.Contains(t, got, "title")
		assert.Contains(t, got, "version")
	})
}

func TestMySQLSink_Close(t *testing.T) {
	sink, _, cleanup := newSink(t)
	defer cleanup()

	assert.NoError(t, sink.Close())
}

func TestCollectColumns(t *testing.T) {
	got := collectColumns([]map[string]any{
		{"id": 1, "title": "a", "deleted_at": 0},
		{"id": 2, "title": "b", "updated_at": 100}, // 多了一个 updated_at
	})
	assert.Equal(t, []string{"deleted_at", "id", "title", "updated_at"}, got, "should dedupe + sort")
}
