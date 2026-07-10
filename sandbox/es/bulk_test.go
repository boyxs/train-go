package es

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── C 批量 Bulk ──────────────────────────────────────────

func TestBulk_Index(t *testing.T) {
	s := freshStore(t)
	ctx := context.Background()
	docs := sampleDocs()

	stats, err := s.BulkIndex(ctx, docs)
	require.NoError(t, err)
	assert.Equal(t, len(docs), stats.Total)
	assert.Equal(t, len(docs), stats.Succeeded)
	assert.Zero(t, stats.Failed)

	cnt, err := s.Count(ctx, nil)
	require.NoError(t, err)
	assert.Equal(t, int64(len(docs)), cnt, "bulk 后总数应匹配")
}

func TestBulk_Mixed(t *testing.T) {
	s := seedStore(t) // 已灌 6 条
	ctx := context.Background()

	stats, err := s.Bulk(ctx, []BulkAction{
		{Op: "index", Id: 100, Doc: Doc{Id: 100, Title: "新品", Category: "electronics", Score: 59, Views: 1, CreatedAt: 1_700_000_000_000}},
		{Op: "update", Id: 1, Doc: map[string]any{"score": 9.9}},
		{Op: "delete", Id: 2},
	})
	require.NoError(t, err)
	assert.Equal(t, 3, stats.Total)
	assert.Equal(t, 3, stats.Succeeded)
	assert.Zero(t, stats.Failed, "混合操作应全部成功: %+v", stats.Failures)

	// 逐个验证操作确实生效
	_, found, err := s.Get(ctx, 100)
	require.NoError(t, err)
	assert.True(t, found, "index 新增应生效")

	d1, _, err := s.Get(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, 9.9, d1.Score, "update 应生效")

	_, found2, err := s.Get(ctx, 2)
	require.NoError(t, err)
	assert.False(t, found2, "delete 应生效")
}

func TestBulk_PartialFailure(t *testing.T) {
	s := freshStore(t)
	ctx := context.Background()
	docs := sampleDocs()
	require.NoError(t, s.Create(ctx, docs[0])) // 先放一条 id=1

	// index id=2（成功）+ create id=1 已存在（409）+ update id=999999 不存在（404）
	stats, err := s.Bulk(ctx, []BulkAction{
		{Op: "index", Id: docs[1].Id, Doc: docs[1]},
		{Op: "create", Id: docs[0].Id, Doc: docs[0]},
		{Op: "update", Id: 999999, Doc: map[string]any{"score": 1.0}},
	})
	require.NoError(t, err, "bulk 整体请求成功，部分失败不整体报错（bulk 语义）")
	assert.Equal(t, 3, stats.Total)
	assert.Equal(t, 1, stats.Succeeded)
	assert.Equal(t, 2, stats.Failed)

	statuses := map[int64]int{}
	for _, f := range stats.Failures {
		statuses[f.Id] = f.Status
	}
	assert.Equal(t, 409, statuses[docs[0].Id], "create 已存在应 409")
	assert.Equal(t, 404, statuses[999999], "update 不存在应 404")
}
