package source

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"github.com/boyxs/train-go/webook/migrator/domain"
	"github.com/boyxs/train-go/webook/pkg/logger"
)

func newFactory(t *testing.T) (SourceFactory, func()) {
	t.Helper()
	return newFactoryWithOpts(t)
}

func newFactoryWithOpts(t *testing.T, opts ...SourceFactoryOption) (SourceFactory, func()) {
	t.Helper()
	mockDB, _, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	gormDB, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      mockDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	require.NoError(t, err)
	return NewSourceFactory(gormDB, logger.NewNopLogger(), opts...), func() { _ = mockDB.Close() }
}

func taskWithTables(tables []domain.TableMapping) domain.Task {
	tj, _ := json.Marshal(tables)
	return domain.Task{Id: 42, Name: "t", TablesJSON: string(tj)}
}

func TestInternalSourceFactory(t *testing.T) {
	t.Run("单表 BuildSrc / BuildDst", func(t *testing.T) {
		f, cleanup := newFactory(t)
		defer cleanup()

		task := taskWithTables([]domain.TableMapping{
			{Src: "article", Dst: "article_v1", PartitionKey: "id"},
		})
		src, err := f.BuildFullSrc(context.Background(), task, 0)
		require.NoError(t, err)
		require.NotNil(t, src)
		dst, err := f.BuildDst(context.Background(), task, 0)
		require.NoError(t, err)
		require.NotNil(t, dst)
	})

	t.Run("PartitionKey 空 → 默认 id", func(t *testing.T) {
		f, cleanup := newFactory(t)
		defer cleanup()

		task := taskWithTables([]domain.TableMapping{
			{Src: "user", Dst: "user_v2"}, // PartitionKey 没填
		})
		src, err := f.BuildFullSrc(context.Background(), task, 0)
		require.NoError(t, err)
		assert.NotNil(t, src)
	})

	t.Run("多表 task：tableIdx=0 取第一张, tableIdx=1 取第二张", func(t *testing.T) {
		f, cleanup := newFactory(t)
		defer cleanup()

		task := taskWithTables([]domain.TableMapping{
			{Src: "article", Dst: "article_v1", PartitionKey: "id"},
			{Src: "comment", Dst: "comment_v1", PartitionKey: "id"},
		})
		src0, err := f.BuildFullSrc(context.Background(), task, 0)
		require.NoError(t, err)
		require.NotNil(t, src0)
		src1, err := f.BuildFullSrc(context.Background(), task, 1)
		require.NoError(t, err)
		require.NotNil(t, src1)
	})

	t.Run("TablesJSON 空 → error", func(t *testing.T) {
		f, cleanup := newFactory(t)
		defer cleanup()
		_, err := f.BuildFullSrc(context.Background(), domain.Task{Id: 1}, 0)
		assert.ErrorContains(t, err, "tables_json is empty")
	})

	t.Run("TablesJSON 损坏 → error", func(t *testing.T) {
		f, cleanup := newFactory(t)
		defer cleanup()
		task := domain.Task{Id: 1, TablesJSON: "not-valid-json"}
		_, err := f.BuildFullSrc(context.Background(), task, 0)
		assert.ErrorContains(t, err, "unmarshal")
	})

	t.Run("TablesJSON [] → error", func(t *testing.T) {
		f, cleanup := newFactory(t)
		defer cleanup()
		task := domain.Task{Id: 1, TablesJSON: "[]"}
		_, err := f.BuildFullSrc(context.Background(), task, 0)
		assert.ErrorContains(t, err, "no entries")
	})

	t.Run("Src 缺失 → error", func(t *testing.T) {
		f, cleanup := newFactory(t)
		defer cleanup()
		task := taskWithTables([]domain.TableMapping{{Dst: "x"}})
		_, err := f.BuildFullSrc(context.Background(), task, 0)
		assert.ErrorContains(t, err, "missing src/dst")
	})
}

func taskWithSource(srcType domain.SourceType, tables []domain.TableMapping) domain.Task {
	tk := taskWithTables(tables)
	tk.SourceType = srcType
	return tk
}

// fakeMongoSource 占位，同时满足 FullSource + IncrSource（仅校验 builder 被调，不真跑方法）。
type fakeMongoSource struct{}

func (*fakeMongoSource) FullScan(context.Context, ShardSpec, chan<- Row) error { return nil }
func (*fakeMongoSource) IncrSubscribe(context.Context, domain.Checkpoint, chan<- ChangeEvent) error {
	return nil
}
func (*fakeMongoSource) Close() error { return nil }

func TestInternalSourceFactory_SourceTypeDispatch(t *testing.T) {
	tables := []domain.TableMapping{{Src: "users", Dst: "user", PartitionKey: "id"}}

	t.Run("源 mysql / 空 → BuildFullSrc 返 *MySQLSource", func(t *testing.T) {
		f, cleanup := newFactory(t)
		defer cleanup()
		src, err := f.BuildFullSrc(context.Background(), taskWithSource("", tables), 0)
		require.NoError(t, err)
		_, ok := src.(*MySQLSource)
		assert.True(t, ok, "空 SourceType 应归一 mysql → *MySQLSource")
	})

	t.Run("源 mongo + 未注入 builder → BuildFullSrc 报错", func(t *testing.T) {
		f, cleanup := newFactory(t)
		defer cleanup()
		_, err := f.BuildFullSrc(context.Background(), taskWithSource(domain.SourceTypeMongo, tables), 0)
		assert.ErrorContains(t, err, "mongo")
	})

	t.Run("源 mongo + 注入 builder → BuildFullSrc 调 builder（带 collection + pkField）", func(t *testing.T) {
		stub := &fakeMongoSource{}
		var gotCollection, gotPK string
		f, cleanup := newFactoryWithOpts(t, WithMongoSourceBuilder(
			func(collection, pkField string) (FullSource, error) {
				gotCollection, gotPK = collection, pkField
				return stub, nil
			}))
		defer cleanup()
		src, err := f.BuildFullSrc(context.Background(), taskWithSource(domain.SourceTypeMongo, tables), 0)
		require.NoError(t, err)
		got, ok := src.(*fakeMongoSource)
		require.True(t, ok)
		assert.Same(t, stub, got)
		assert.Equal(t, "users", gotCollection)
		assert.Equal(t, "id", gotPK)
	})

	t.Run("源 mongo + 未注入 incr builder → BuildIncrSrc 报错", func(t *testing.T) {
		f, cleanup := newFactory(t)
		defer cleanup()
		_, err := f.BuildIncrSrc(context.Background(), taskWithSource(domain.SourceTypeMongo, tables), 0)
		assert.ErrorContains(t, err, "not configured")
	})

	t.Run("源 mongo + 注入 incr builder → BuildIncrSrc 调 builder", func(t *testing.T) {
		stub := &fakeMongoSource{}
		f, cleanup := newFactoryWithOpts(t, WithMongoIncrSourceBuilder(
			func(collection, _ string) (IncrSource, error) {
				assert.Equal(t, "users", collection)
				return stub, nil
			}))
		defer cleanup()
		src, err := f.BuildIncrSrc(context.Background(), taskWithSource(domain.SourceTypeMongo, tables), 0)
		require.NoError(t, err)
		got, ok := src.(*fakeMongoSource)
		require.True(t, ok)
		assert.Same(t, stub, got)
	})
}
