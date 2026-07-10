package es

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── B 文档 CRUD ──────────────────────────────────────────

func TestIndex_And_Get(t *testing.T) {
	s := freshStore(t)
	ctx := context.Background()
	d := sampleDocs()[0]
	require.NoError(t, s.Index(ctx, d))

	got, found, err := s.Get(ctx, d.Id)
	require.NoError(t, err)
	require.True(t, found, "写入后应能取到")
	assert.Equal(t, d, got, "取回文档应与写入完全一致")
}

func TestCreate_Conflict(t *testing.T) {
	s := freshStore(t)
	ctx := context.Background()
	d := sampleDocs()[0]
	require.NoError(t, s.Create(ctx, d), "首次 Create 应成功")

	err := s.Create(ctx, d)
	require.Error(t, err, "重复 Create 同一 ID 应失败")
	assert.True(t, IsConflict(err), "应为 409 冲突, got: %v", err)
}

func TestGet_AllFieldTypes(t *testing.T) {
	s := freshStore(t)
	ctx := context.Background()
	d := sampleDocs()[2] // furniture chair
	require.NoError(t, s.Index(ctx, d))

	got, found, err := s.Get(ctx, d.Id)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "furniture", got.Category)                 // keyword
	assert.Equal(t, []string{"office", "ergonomic"}, got.Tags) // keyword 多值
	assert.Equal(t, 1299.0, got.Score)                         // double
	assert.Equal(t, 10, got.Views)                             // integer
	assert.Equal(t, d.CreatedAt, got.CreatedAt)                // date epoch_millis
}

func TestGet_NotFound(t *testing.T) {
	s := freshStore(t)
	_, found, err := s.Get(context.Background(), 999999)
	require.NoError(t, err, "取不存在文档不应报错")
	assert.False(t, found)
}

func TestUpdate_Partial(t *testing.T) {
	s := freshStore(t)
	ctx := context.Background()
	d := sampleDocs()[0]
	require.NoError(t, s.Index(ctx, d))

	require.NoError(t, s.Update(ctx, d.Id, map[string]any{"score": 149.0, "views": 88}))

	got, found, err := s.Get(ctx, d.Id)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, 149.0, got.Score, "score 应更新")
	assert.Equal(t, 88, got.Views, "views 应更新")
	assert.Equal(t, d.Title, got.Title, "未更新字段应保留")
	assert.Equal(t, d.Category, got.Category, "未更新字段应保留")
}

func TestUpdate_NotFound(t *testing.T) {
	s := freshStore(t)
	err := s.Update(context.Background(), 999999, map[string]any{"score": 1.0})
	require.Error(t, err, "更新不存在文档应失败")
	assert.True(t, IsNotFound(err), "应为 404, got: %v", err)
}

func TestDelete(t *testing.T) {
	s := freshStore(t)
	ctx := context.Background()
	d := sampleDocs()[0]
	require.NoError(t, s.Index(ctx, d))

	deleted, err := s.Delete(ctx, d.Id)
	require.NoError(t, err)
	assert.True(t, deleted, "应删除成功")

	_, found, err := s.Get(ctx, d.Id)
	require.NoError(t, err)
	assert.False(t, found, "删除后不应存在")

	// 删不存在的文档：不报错、返回 false
	deleted, err = s.Delete(ctx, d.Id)
	require.NoError(t, err)
	assert.False(t, deleted, "删不存在文档应返回 false")
}

func TestDocExists(t *testing.T) {
	s := freshStore(t)
	ctx := context.Background()
	d := sampleDocs()[0]

	exists, err := s.DocExists(ctx, d.Id)
	require.NoError(t, err)
	assert.False(t, exists, "写入前不存在")

	require.NoError(t, s.Index(ctx, d))
	exists, err = s.DocExists(ctx, d.Id)
	require.NoError(t, err)
	assert.True(t, exists, "写入后存在")
}
