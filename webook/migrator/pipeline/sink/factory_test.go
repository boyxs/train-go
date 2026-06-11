package sink

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"github.com/webook/migrator/domain"
	"github.com/webook/pkg/logger"
)

func newSinkFactory(t *testing.T) (SinkFactory, func()) {
	t.Helper()
	mockDB, _, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	gormDB, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      mockDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	require.NoError(t, err)
	return NewSinkFactory(gormDB, logger.NewNopLogger()), func() { _ = mockDB.Close() }
}

func taskWithTables(tables []domain.TableMapping) domain.Task {
	tj, _ := json.Marshal(tables)
	return domain.Task{Id: 42, Name: "t", TablesJSON: string(tj)}
}

func TestInternalSinkFactory(t *testing.T) {
	t.Run("单表 BuildSrc / BuildDst", func(t *testing.T) {
		f, cleanup := newSinkFactory(t)
		defer cleanup()

		task := taskWithTables([]domain.TableMapping{
			{Src: "article", Dst: "article_v1", PartitionKey: "id"},
		})
		src, err := f.BuildSrc(context.Background(), task, 0)
		require.NoError(t, err)
		require.NotNil(t, src)
		dst, err := f.BuildDst(context.Background(), task, 0)
		require.NoError(t, err)
		require.NotNil(t, dst)
	})

	t.Run("TablesJSON 空 → error", func(t *testing.T) {
		f, cleanup := newSinkFactory(t)
		defer cleanup()
		_, err := f.BuildDst(context.Background(), domain.Task{Id: 1}, 0)
		assert.ErrorContains(t, err, "tables_json is empty")
	})

	t.Run("Dst 缺失 → error", func(t *testing.T) {
		f, cleanup := newSinkFactory(t)
		defer cleanup()
		task := taskWithTables([]domain.TableMapping{{Src: "x"}})
		_, err := f.BuildDst(context.Background(), task, 0)
		assert.ErrorContains(t, err, "missing src/dst")
	})
}
