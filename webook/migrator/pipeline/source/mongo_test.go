package source

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/boyxs/train-go/webook/migrator/domain"
	"github.com/boyxs/train-go/webook/pkg/logger"
)

// fakeScanner 喂 canned 文档，绕开真 Mongo（驱动逻辑由真 goMongoScanner 在 e2e 覆盖）。
type fakeScanner struct {
	docs []map[string]any
	err  error
}

func (f *fakeScanner) Scan(_ context.Context, _ int, fn func(doc map[string]any) error) error {
	for _, d := range f.docs {
		if err := fn(d); err != nil {
			return err
		}
	}
	return f.err
}

func TestMongoSource_FullScan(t *testing.T) {
	t.Run("逐文档 → Row{PK:_id, Table:collection, Cols:doc}", func(t *testing.T) {
		src := NewMongoSource(&fakeScanner{docs: []map[string]any{
			{"_id": "65a", "name": "alice"},
			{"_id": "65b", "name": "bob"},
		}}, "users", logger.NewNopLogger())

		out := make(chan Row, 8)
		err := src.FullScan(context.Background(), ShardSpec{BatchSz: 100}, out)
		require.NoError(t, err)
		close(out)

		var got []Row
		for r := range out {
			got = append(got, r)
		}
		require.Len(t, got, 2)
		assert.Equal(t, "65a", got[0].PK)
		assert.Equal(t, "users", got[0].Table)
		assert.Equal(t, "alice", got[0].Cols["name"])
		assert.Equal(t, "65b", got[1].PK)
	})

	t.Run("文档缺 _id → error", func(t *testing.T) {
		src := NewMongoSource(&fakeScanner{docs: []map[string]any{
			{"name": "no-id"},
		}}, "users", logger.NewNopLogger())
		out := make(chan Row, 8)
		err := src.FullScan(context.Background(), ShardSpec{}, out)
		assert.ErrorContains(t, err, "_id")
	})

	t.Run("scanner 出错向上传播", func(t *testing.T) {
		boom := errors.New("mongo down")
		src := NewMongoSource(&fakeScanner{err: boom}, "users", logger.NewNopLogger())
		out := make(chan Row, 8)
		err := src.FullScan(context.Background(), ShardSpec{}, out)
		assert.ErrorIs(t, err, boom)
	})
}

func TestNormalizeBSONMap(t *testing.T) {
	oid, err := primitive.ObjectIDFromHex("507f1f77bcf86cd799439011")
	require.NoError(t, err)
	out := normalizeBSONMap(bson.M{
		"_id":  oid,
		"name": "alice",
		"age":  int64(30),
		"meta": bson.M{"city": "SG"}, // 嵌套 bson.M → map[string]any
		"tags": bson.A{"go", "db"},   // bson.A → []any
		"ts":   primitive.DateTime(1700000000000),
	})
	assert.Equal(t, "507f1f77bcf86cd799439011", out["_id"]) // ObjectID → hex
	assert.Equal(t, "alice", out["name"])
	assert.Equal(t, int64(30), out["age"])
	assert.Equal(t, map[string]any{"city": "SG"}, out["meta"])
	assert.Equal(t, []any{"go", "db"}, out["tags"])
	assert.Equal(t, int64(1700000000000), out["ts"]) // DateTime → Unix 毫秒
}

// fakeWatcher 喂 canned change 事件，绕开真 Mongo Change Stream（真 goMongoWatcher 由 e2e 覆盖）。
type fakeWatcher struct {
	events    []mongoChangeEvent
	gotResume string
}

func (f *fakeWatcher) Watch(_ context.Context, resumeToken string, fn func(ev mongoChangeEvent) error) error {
	f.gotResume = resumeToken
	for _, ev := range f.events {
		if err := fn(ev); err != nil {
			return err
		}
	}
	return nil
}

func TestMongoIncrSource_IncrSubscribe(t *testing.T) {
	w := &fakeWatcher{events: []mongoChangeEvent{
		{Op: "insert", ID: "u1", FullDoc: map[string]any{"_id": "u1", "name": "alice"}, ResumeToken: "tok1", ClusterTime: 1000},
		{Op: "delete", ID: "u2", ResumeToken: "tok2", ClusterTime: 2000},
	}}
	src := NewMongoIncrSource(w, "users", logger.NewNopLogger())

	out := make(chan ChangeEvent, 8)
	err := src.IncrSubscribe(context.Background(), domain.Checkpoint{CursorValue: "tok0"}, out)
	require.NoError(t, err)
	close(out)

	var got []ChangeEvent
	for ev := range out {
		got = append(got, ev)
	}
	require.Len(t, got, 2)
	assert.Equal(t, "tok0", w.gotResume) // 从 ckpt 的 resume token 续订
	assert.Equal(t, "insert", got[0].Op)
	assert.Equal(t, "u1", got[0].PK)
	assert.Equal(t, "alice", got[0].After["name"])
	assert.Equal(t, "tok1", got[0].BinlogPos) // resume token 进 BinlogPos（引擎当游标持久化）
	assert.Equal(t, int64(1000), got[0].EventTs)
	assert.Equal(t, "delete", got[1].Op)
	assert.Equal(t, "u2", got[1].PK)
}

func TestNormalizeChangeOp(t *testing.T) {
	assert.Equal(t, "insert", normalizeChangeOp("insert"))
	assert.Equal(t, "update", normalizeChangeOp("update"))
	assert.Equal(t, "update", normalizeChangeOp("replace")) // replace 归一为 update（整文档 upsert）
	assert.Equal(t, "delete", normalizeChangeOp("delete"))
	assert.Equal(t, "", normalizeChangeOp("drop")) // 不关心的事件 → 空（跳过）
}
