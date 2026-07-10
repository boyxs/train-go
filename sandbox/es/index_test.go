package es

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── A 索引管理 ───────────────────────────────────────────

func TestCreateIndex_WithMapping(t *testing.T) {
	s := freshStore(t)
	ctx := context.Background()

	exists, err := s.IndexExists(ctx)
	require.NoError(t, err)
	assert.True(t, exists, "CreateIndex 后索引应存在")

	m, err := s.GetMapping(ctx)
	require.NoError(t, err)
	assert.Equal(t, "keyword", mappingFieldType(t, m, s.index, "category"), "category 应为 keyword")
	assert.Equal(t, "double", mappingFieldType(t, m, s.index, "score"), "score 应为 double")
	assert.Equal(t, "date", mappingFieldType(t, m, s.index, "created_at"), "created_at 应为 date")
}

func TestIndexExists(t *testing.T) {
	s := freshStore(t)
	ctx := context.Background()

	exists, err := s.IndexExists(ctx)
	require.NoError(t, err)
	assert.True(t, exists, "已建索引应存在")

	other := NewDocStore(s.client, "es_demo_absent_index")
	exists, err = other.IndexExists(ctx)
	require.NoError(t, err)
	assert.False(t, exists, "未建索引应不存在")
}

func TestGetMapping(t *testing.T) {
	s := freshStore(t)
	m, err := s.GetMapping(context.Background())
	require.NoError(t, err)
	// text / keyword / long / integer 各类型都应如实回读
	assert.Equal(t, "text", mappingFieldType(t, m, s.index, "title"))
	assert.Equal(t, "keyword", mappingFieldType(t, m, s.index, "tags"))
	assert.Equal(t, "long", mappingFieldType(t, m, s.index, "id"))
	assert.Equal(t, "integer", mappingFieldType(t, m, s.index, "views"))
}

func TestDeleteIndex(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	require.NoError(t, s.DeleteIndex(ctx)) // 前置清理
	require.NoError(t, s.CreateIndex(ctx))

	exists, err := s.IndexExists(ctx)
	require.NoError(t, err)
	require.True(t, exists)

	require.NoError(t, s.DeleteIndex(ctx), "删索引")
	exists, err = s.IndexExists(ctx)
	require.NoError(t, err)
	assert.False(t, exists, "删除后索引应不存在")

	// 幂等：再删不存在的索引不报错
	assert.NoError(t, s.DeleteIndex(ctx), "重复删（404）应幂等")
}
