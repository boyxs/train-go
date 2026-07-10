package es

import (
	"context"
	"testing"

	"github.com/elastic/go-elasticsearch/v9/typedapi/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── E 聚合 ───────────────────────────────────────────────

func TestAgg_TermsCount(t *testing.T) {
	s := seedStore(t)
	counts, err := s.TermsCount(context.Background(), "category")
	require.NoError(t, err)
	assert.Equal(t, map[string]int64{"electronics": 3, "furniture": 2, "watch": 1}, counts)
}

func TestAgg_Stats(t *testing.T) {
	s := seedStore(t)
	st, err := s.Stats(context.Background(), "score")
	require.NoError(t, err)
	assert.Equal(t, int64(6), st.Count)
	assert.Equal(t, 199.0, st.Min)
	assert.Equal(t, 8999.0, st.Max)
	assert.Equal(t, 11994.0, st.Sum)
	assert.Equal(t, 1999.0, st.Avg)
}

func TestAgg_Nested(t *testing.T) {
	s := seedStore(t)
	// 先按 category 分组，组内再求均分 —— 演示直接读 SearchResult.Aggs 解析嵌套聚合
	res, err := s.Search(context.Background(), map[string]any{
		"size": 0,
		"aggs": map[string]any{
			"by_cat": map[string]any{
				"terms": map[string]any{"field": "category"},
				"aggs":  map[string]any{"avg_score": map[string]any{"avg": map[string]any{"field": "score"}}},
			},
		},
	})
	require.NoError(t, err)

	buckets, err := stringTermsBuckets(res.Aggs["by_cat"])
	require.NoError(t, err)
	avgByCat := map[string]float64{}
	for _, b := range buckets {
		key, ok := b.Key.(string)
		require.True(t, ok, "桶 key 应为 string, got %T", b.Key)
		sub, ok := b.Aggregations["avg_score"].(*types.AvgAggregate)
		require.True(t, ok, "嵌套 avg 应为 *AvgAggregate, got %T", b.Aggregations["avg_score"])
		require.NotNil(t, sub.Value)
		avgByCat[key] = float64(*sub.Value)
	}
	assert.Equal(t, 299.0, avgByCat["electronics"], "(199+399+299)/3")
	assert.Equal(t, 1049.0, avgByCat["furniture"], "(1299+799)/2")
	assert.Equal(t, 8999.0, avgByCat["watch"])
}
