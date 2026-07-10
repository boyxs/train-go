package es

import (
	"context"
	"fmt"

	"github.com/elastic/go-elasticsearch/v9/typedapi/types"
)

// ── E 聚合（强类型解析封装）──────────────────────────────
// Search 已设 TypedKeys(true)，聚合结果带类型前缀，可解析成强类型 Aggregate union。
// 这两个封装隐藏了 union 类型断言的细节；更复杂的嵌套聚合可直接读 SearchResult.Aggs。

// StatsResult stats 聚合结果（min/max/avg/sum/count）。
type StatsResult struct {
	Count int64
	Min   float64
	Max   float64
	Avg   float64
	Sum   float64
}

// TermsCount 对 keyword 字段做 terms 分组计数，返回 key→doc_count。
func (s *DocStore) TermsCount(ctx context.Context, field string) (map[string]int64, error) {
	res, err := s.Search(ctx, map[string]any{
		"size": 0, // 只要聚合，不要文档
		"aggs": map[string]any{"g": map[string]any{"terms": map[string]any{"field": field}}},
	})
	if err != nil {
		return nil, err
	}
	buckets, err := stringTermsBuckets(res.Aggs["g"])
	if err != nil {
		return nil, err
	}
	out := make(map[string]int64, len(buckets))
	for _, b := range buckets {
		key, ok := b.Key.(string)
		if !ok {
			return nil, fmt.Errorf("terms 桶 key 非 string: %T", b.Key)
		}
		out[key] = b.DocCount
	}
	return out, nil
}

// Stats 对数值字段做 stats 聚合。
func (s *DocStore) Stats(ctx context.Context, field string) (StatsResult, error) {
	res, err := s.Search(ctx, map[string]any{
		"size": 0,
		"aggs": map[string]any{"s": map[string]any{"stats": map[string]any{"field": field}}},
	})
	if err != nil {
		return StatsResult{}, err
	}
	agg, ok := res.Aggs["s"].(*types.StatsAggregate)
	if !ok {
		return StatsResult{}, fmt.Errorf("stats agg 类型非预期: %T", res.Aggs["s"])
	}
	out := StatsResult{Count: agg.Count, Sum: float64(agg.Sum)}
	if agg.Min != nil {
		out.Min = float64(*agg.Min)
	}
	if agg.Max != nil {
		out.Max = float64(*agg.Max)
	}
	if agg.Avg != nil {
		out.Avg = float64(*agg.Avg)
	}
	return out, nil
}

// stringTermsBuckets 把 terms 聚合结果断言成字符串桶切片（供 TermsCount 及测试的嵌套聚合复用）。
func stringTermsBuckets(agg types.Aggregate) ([]types.StringTermsBucket, error) {
	st, ok := agg.(*types.StringTermsAggregate)
	if !ok {
		return nil, fmt.Errorf("terms agg 类型非预期: %T", agg)
	}
	buckets, ok := st.Buckets.([]types.StringTermsBucket)
	if !ok {
		return nil, fmt.Errorf("terms buckets 类型非预期: %T", st.Buckets)
	}
	return buckets, nil
}
