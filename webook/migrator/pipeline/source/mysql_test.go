package source

import (
	"context"
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"github.com/boyxs/train-go/webook/pkg/logger"
)

// newSource 启动 sqlmock + GORM，返回 source + mock + 一个 cleanup。
func newSource(t *testing.T) (FullSource, sqlmock.Sqlmock, func()) {
	t.Helper()
	mockDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	gormDB, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      mockDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	require.NoError(t, err)
	src := NewMySQLSource(gormDB, "article", "id", logger.NewNopLogger())
	return src, mock, func() { _ = mockDB.Close() }
}

func TestMySQLSource_FullScan(t *testing.T) {
	t.Run("扫满 BatchSz 后第二批返回空 → 结束", func(t *testing.T) {
		src, mock, cleanup := newSource(t)
		defer cleanup()

		// 第一批：返回 2 行（< BatchSz=10 直接结束）
		mock.ExpectQuery("SELECT * FROM `article` WHERE `id` > ? AND `id` <= ? ORDER BY `id` ASC LIMIT ?").
			WithArgs(int64(0), int64(100), 10).
			WillReturnRows(sqlmock.NewRows([]string{"id", "title"}).
				AddRow(int64(1), "a").
				AddRow(int64(2), "b"))

		out := make(chan Row, 16)
		err := src.FullScan(context.Background(), ShardSpec{No: 0, PKMin: 1, PKMax: 100, BatchSz: 10}, out)
		assert.NoError(t, err)
		close(out)

		var got []Row
		for r := range out {
			got = append(got, r)
		}
		require.Len(t, got, 2)
		assert.Equal(t, "1", got[0].PK)
		assert.Equal(t, "article", got[0].Table)
		assert.Equal(t, "2", got[1].PK)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("多批：第一批满 → 第二批不满 → 结束", func(t *testing.T) {
		src, mock, cleanup := newSource(t)
		defer cleanup()

		// 第一批：2 行（BatchSz=2 → 满批）
		mock.ExpectQuery("SELECT * FROM `article` WHERE `id` > ? AND `id` <= ? ORDER BY `id` ASC LIMIT ?").
			WithArgs(int64(0), int64(100), 2).
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(1)).AddRow(int64(2)))
		// 第二批：1 行（不满）
		mock.ExpectQuery("SELECT * FROM `article` WHERE `id` > ? AND `id` <= ? ORDER BY `id` ASC LIMIT ?").
			WithArgs(int64(2), int64(100), 2).
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(3)))

		out := make(chan Row, 16)
		err := src.FullScan(context.Background(), ShardSpec{PKMin: 1, PKMax: 100, BatchSz: 2}, out)
		assert.NoError(t, err)
		close(out)

		var pks []string
		for r := range out {
			pks = append(pks, r.PK)
		}
		assert.Equal(t, []string{"1", "2", "3"}, pks)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("空表 → 一批空结果直接结束", func(t *testing.T) {
		src, mock, cleanup := newSource(t)
		defer cleanup()

		mock.ExpectQuery("SELECT * FROM `article` WHERE `id` > ? AND `id` <= ? ORDER BY `id` ASC LIMIT ?").
			WithArgs(int64(0), int64(100), 1000).
			WillReturnRows(sqlmock.NewRows([]string{"id"}))

		out := make(chan Row, 1)
		err := src.FullScan(context.Background(), ShardSpec{PKMin: 1, PKMax: 100}, out)
		assert.NoError(t, err)
		close(out)
		_, more := <-out
		assert.False(t, more, "out should be empty for empty table")
	})

	t.Run("DB 错误向上传播", func(t *testing.T) {
		src, mock, cleanup := newSource(t)
		defer cleanup()

		mock.ExpectQuery("SELECT * FROM `article` WHERE `id` > ? AND `id` <= ? ORDER BY `id` ASC LIMIT ?").
			WillReturnError(sql.ErrConnDone)

		out := make(chan Row, 1)
		err := src.FullScan(context.Background(), ShardSpec{PKMin: 1, PKMax: 100}, out)
		assert.ErrorIs(t, err, sql.ErrConnDone)
	})

	t.Run("ctx 取消 → 提前返回 ctx.Err", func(t *testing.T) {
		src, mock, cleanup := newSource(t)
		defer cleanup()

		ctx, cancel := context.WithCancel(context.Background())
		// 模拟一批返回，进入 channel send 阶段；out 不消费时阻塞
		mock.ExpectQuery("SELECT * FROM `article` WHERE `id` > ? AND `id` <= ? ORDER BY `id` ASC LIMIT ?").
			WithArgs(int64(0), int64(100), 1000).
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(1)))
		out := make(chan Row) // 不缓冲，send 会阻塞
		cancel()              // 提前取消
		err := src.FullScan(ctx, ShardSpec{PKMin: 1, PKMax: 100}, out)
		assert.Error(t, err)
	})
}

func TestMySQLSource_PKRange(t *testing.T) {
	t.Run("非空表 → 正确返 min/max", func(t *testing.T) {
		src, mock, cleanup := newSource(t)
		defer cleanup()
		mock.ExpectQuery("SELECT MIN(`id`) AS min_pk, MAX(`id`) AS max_pk FROM `article` LIMIT ?").
			WithArgs(1).
			WillReturnRows(sqlmock.NewRows([]string{"min_pk", "max_pk"}).AddRow(int64(1), int64(99)))

		ranger, ok := src.(PKRanger)
		require.True(t, ok, "MySQLSource 必须实现 PKRanger")
		minPK, maxPK, err := ranger.PKRange(context.Background())
		require.NoError(t, err)
		assert.Equal(t, int64(1), minPK)
		assert.Equal(t, int64(99), maxPK)
	})

	t.Run("空表 → (0, 0, nil)", func(t *testing.T) {
		src, mock, cleanup := newSource(t)
		defer cleanup()
		mock.ExpectQuery("SELECT MIN(`id`) AS min_pk, MAX(`id`) AS max_pk FROM `article` LIMIT ?").
			WithArgs(1).
			WillReturnRows(sqlmock.NewRows([]string{"min_pk", "max_pk"}).AddRow(nil, nil))

		minPK, maxPK, err := src.(PKRanger).PKRange(context.Background())
		require.NoError(t, err)
		assert.Equal(t, int64(0), minPK)
		assert.Equal(t, int64(0), maxPK)
	})

	t.Run("DB 错误向上传播", func(t *testing.T) {
		src, mock, cleanup := newSource(t)
		defer cleanup()
		mock.ExpectQuery("SELECT MIN(`id`) AS min_pk, MAX(`id`) AS max_pk FROM `article` LIMIT ?").
			WillReturnError(sql.ErrConnDone)

		_, _, err := src.(PKRanger).PKRange(context.Background())
		assert.ErrorIs(t, err, sql.ErrConnDone)
	})
}

func TestMySQLSource_Close(t *testing.T) {
	t.Run("Close no-op", func(t *testing.T) {
		src, _, cleanup := newSource(t)
		defer cleanup()

		assert.NoError(t, src.Close())
	})
}

func TestToInt64(t *testing.T) {
	testCases := []struct {
		name string
		in   any
		want int64
		ok   bool
	}{
		{"int64", int64(42), 42, true},
		{"int", 42, 42, true},
		{"uint64", uint64(42), 42, true},
		{"[]byte", []byte("42"), 42, true},
		{"string", "42", 42, true},
		{"非数字 string", "abc", 0, false},
		{"float64", 42.0, 0, false},
		{"nil", nil, 0, false},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := toInt64(c.in)
			assert.Equal(t, c.want, got)
			assert.Equal(t, c.ok, ok)
		})
	}
}
