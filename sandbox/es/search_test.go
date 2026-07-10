package es

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── D 搜索 ───────────────────────────────────────────────

func TestSearch_MatchAll(t *testing.T) {
	s := seedStore(t)
	r, err := s.Search(context.Background(), map[string]any{
		"query": map[string]any{"match_all": map[string]any{}},
	})
	require.NoError(t, err)
	assert.Equal(t, int64(6), r.Total, "全量命中数")
	assert.Len(t, r.Hits, 6)
}

func TestSearch_Match_Analyzed(t *testing.T) {
	s := seedStore(t)
	// content 是 text，match 会分词；"wireless" 命中 doc1、doc5
	r, err := s.Search(context.Background(), map[string]any{
		"query": map[string]any{"match": map[string]any{"content": "wireless"}},
	})
	require.NoError(t, err)
	assert.ElementsMatch(t, []int64{1, 5}, hitIds(r))
}

func TestSearch_Term_Exact(t *testing.T) {
	s := seedStore(t)
	// category 是 keyword，term 精确匹配；electronics = doc1,2,5
	r, err := s.Search(context.Background(), map[string]any{
		"query": map[string]any{"term": map[string]any{"category": "electronics"}},
	})
	require.NoError(t, err)
	assert.ElementsMatch(t, []int64{1, 2, 5}, hitIds(r))
}

func TestSearch_Terms_MultiValue(t *testing.T) {
	s := seedStore(t)
	r, err := s.Search(context.Background(), map[string]any{
		"query": map[string]any{"terms": map[string]any{"category": []string{"electronics", "watch"}}},
	})
	require.NoError(t, err)
	assert.ElementsMatch(t, []int64{1, 2, 4, 5}, hitIds(r))
}

func TestSearch_Bool_Must_Filter_MustNot(t *testing.T) {
	s := seedStore(t)
	// must: content 含 office(doc3,6) ; filter: category=furniture(3,6) ; must_not: tags=ergonomic(排除3) → {6}
	r, err := s.Search(context.Background(), map[string]any{
		"query": map[string]any{
			"bool": map[string]any{
				"must":     []any{map[string]any{"match": map[string]any{"content": "office"}}},
				"filter":   []any{map[string]any{"term": map[string]any{"category": "furniture"}}},
				"must_not": []any{map[string]any{"term": map[string]any{"tags": "ergonomic"}}},
			},
		},
	})
	require.NoError(t, err)
	assert.ElementsMatch(t, []int64{6}, hitIds(r))
}

func TestSearch_Range(t *testing.T) {
	s := seedStore(t)
	ctx := context.Background()

	// 分值区间 [300,1000]：doc2(399)、doc6(799)
	r, err := s.Search(ctx, map[string]any{
		"query": map[string]any{"range": map[string]any{"score": map[string]any{"gte": 300, "lte": 1000}}},
	})
	require.NoError(t, err)
	assert.ElementsMatch(t, []int64{2, 6}, hitIds(r), "score 范围")

	// 时间范围：created_at >= base+3day → doc4,5,6
	const base = int64(1_700_000_000_000)
	const day = int64(24 * 60 * 60 * 1000)
	r, err = s.Search(ctx, map[string]any{
		"query": map[string]any{"range": map[string]any{"created_at": map[string]any{"gte": base + 3*day}}},
	})
	require.NoError(t, err)
	assert.ElementsMatch(t, []int64{4, 5, 6}, hitIds(r), "时间范围")
}

func TestSearch_Paging(t *testing.T) {
	s := seedStore(t)
	ctx := context.Background()
	sortByScoreAsc := []any{map[string]any{"score": map[string]any{"order": "asc"}}}

	// 第 1 页：分值最低两条 → 199(1)、299(5)
	r, err := s.Search(ctx, map[string]any{"from": 0, "size": 2, "sort": sortByScoreAsc, "query": map[string]any{"match_all": map[string]any{}}})
	require.NoError(t, err)
	assert.Equal(t, []int64{1, 5}, hitIds(r), "第1页")
	assert.Equal(t, int64(6), r.Total, "total 仍是全量")

	// 第 2 页 → 399(2)、799(6)
	r, err = s.Search(ctx, map[string]any{"from": 2, "size": 2, "sort": sortByScoreAsc, "query": map[string]any{"match_all": map[string]any{}}})
	require.NoError(t, err)
	assert.Equal(t, []int64{2, 6}, hitIds(r), "第2页")
}

func TestSearch_Sort(t *testing.T) {
	s := seedStore(t)
	ctx := context.Background()

	r, err := s.Search(ctx, map[string]any{
		"sort":  []any{map[string]any{"score": map[string]any{"order": "asc"}}},
		"query": map[string]any{"match_all": map[string]any{}},
	})
	require.NoError(t, err)
	assert.Equal(t, []int64{1, 5, 2, 6, 3, 4}, hitIds(r), "score 升序")

	r, err = s.Search(ctx, map[string]any{
		"sort":  []any{map[string]any{"score": map[string]any{"order": "desc"}}},
		"query": map[string]any{"match_all": map[string]any{}},
	})
	require.NoError(t, err)
	assert.Equal(t, []int64{4, 3, 6, 2, 5, 1}, hitIds(r), "score 降序")
}

func TestSearch_Highlight(t *testing.T) {
	s := seedStore(t)
	r, err := s.Search(context.Background(), map[string]any{
		"query":     map[string]any{"match": map[string]any{"content": "wireless"}},
		"highlight": map[string]any{"fields": map[string]any{"content": map[string]any{}}},
	})
	require.NoError(t, err)
	require.NotEmpty(t, r.Hits)
	for _, h := range r.Hits {
		frags := h.Highlight["content"]
		require.NotEmpty(t, frags, "命中应带 content 高亮片段")
		assert.Contains(t, frags[0], "<em>wireless</em>", "高亮词应被 <em> 包裹")
	}
}

func TestSearch_SourceFilter(t *testing.T) {
	s := seedStore(t)
	r, err := s.Search(context.Background(), map[string]any{
		"_source": []string{"id", "title"},
		"query":   map[string]any{"term": map[string]any{"category": "watch"}}, // doc4
	})
	require.NoError(t, err)
	require.Len(t, r.Hits, 1)
	got := r.Hits[0].Doc
	assert.Equal(t, int64(4), got.Id, "id 应返回")
	assert.NotEmpty(t, got.Title, "title 应返回")
	assert.Empty(t, got.Category, "未选字段 category 不应返回")
	assert.Zero(t, got.Score, "未选字段 score 不应返回")
}

func TestSearch_EmptyResult(t *testing.T) {
	s := seedStore(t)
	r, err := s.Search(context.Background(), map[string]any{
		"query": map[string]any{"term": map[string]any{"category": "does_not_exist"}},
	})
	require.NoError(t, err)
	assert.Equal(t, int64(0), r.Total)
	assert.Empty(t, r.Hits)
}

// ── F 计数 ───────────────────────────────────────────────

func TestCount_All(t *testing.T) {
	s := seedStore(t)
	cnt, err := s.Count(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, int64(6), cnt)
}

func TestCount_WithQuery(t *testing.T) {
	s := seedStore(t)
	cnt, err := s.Count(context.Background(), map[string]any{
		"query": map[string]any{"term": map[string]any{"category": "electronics"}},
	})
	require.NoError(t, err)
	assert.Equal(t, int64(3), cnt)
}
