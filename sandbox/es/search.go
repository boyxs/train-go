package es

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/elastic/go-elasticsearch/v9/typedapi/core/search"
)

// ── D 搜索 / F 计数 ──────────────────────────────────────

// Search 用 map[string]any 查询体检索（body 即 ES 查询 DSL：query/from/size/sort/highlight/
// aggs/_source/collapse 等）。TypedKeys(true) 让聚合结果带类型前缀，才能解析成强类型 Aggregate。
func (s *DocStore) Search(ctx context.Context, body map[string]any) (SearchResult, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return SearchResult{}, fmt.Errorf("marshal search body: %w", err)
	}
	resp, err := s.client.Search().Index(s.index).Raw(bytes.NewReader(raw)).TypedKeys(true).Do(ctx)
	if err != nil {
		return SearchResult{}, err
	}
	return toSearchResult(resp)
}

// Count 统计命中数；body 为 nil 时统计全部文档。
func (s *DocStore) Count(ctx context.Context, body map[string]any) (int64, error) {
	req := s.client.Count().Index(s.index)
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return 0, fmt.Errorf("marshal count body: %w", err)
		}
		req = req.Raw(bytes.NewReader(raw))
	}
	resp, err := req.Do(ctx)
	if err != nil {
		return 0, err
	}
	return resp.Count, nil
}

// toSearchResult 把 TypedClient 搜索响应转成 SearchResult（搜索 + search_after 复用）。
func toSearchResult(resp *search.Response) (SearchResult, error) {
	out := SearchResult{Aggs: resp.Aggregations}
	if resp.Hits.Total != nil {
		out.Total = resp.Hits.Total.Value
	}
	for _, h := range resp.Hits.Hits {
		d, err := decodeDoc(h.Source_)
		if err != nil {
			return SearchResult{}, err
		}
		hit := SearchHit{Doc: d, Highlight: h.Highlight}
		if h.Score_ != nil {
			hit.Score = float64(*h.Score_)
		}
		for _, sv := range h.Sort {
			hit.Sort = append(hit.Sort, sv)
		}
		out.Hits = append(out.Hits, hit)
	}
	return out, nil
}
