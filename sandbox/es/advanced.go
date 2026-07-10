package es

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/elastic/go-elasticsearch/v9/typedapi/types"
)

// ── 高级①：Point-in-Time + search_after 深分页 ───────────

// OpenPIT 打开 Point-in-Time 快照，返回 pit id（翻页期间锁定一致的数据视图，
// 避免深分页过程中数据变更导致漏读/重读）。keepAlive 如 "1m"。
func (s *DocStore) OpenPIT(ctx context.Context, keepAlive string) (string, error) {
	resp, err := s.client.OpenPointInTime(s.index).KeepAlive(keepAlive).Do(ctx)
	if err != nil {
		return "", err
	}
	return resp.Id, nil
}

// ClosePIT 关闭 PIT，释放资源。
func (s *DocStore) ClosePIT(ctx context.Context, pitId string) error {
	_, err := s.client.ClosePointInTime().Id(pitId).Do(ctx)
	return err
}

// SearchAfter 基于 PIT 的 search_after 深分页：after 传上一页最后一条的 sort 值（nil=首页），
// 返回本页结果 + 本页最后一条的 sort 值（作为下一页的 after；本页为空则返 nil）。
// 注意：走 PIT 时不指定 index（PIT 已绑定索引）。
func (s *DocStore) SearchAfter(ctx context.Context, pitId string, size int, sort []any, after []any) (SearchResult, []any, error) {
	body := map[string]any{
		"size": size,
		"sort": sort,
		"pit":  map[string]any{"id": pitId, "keep_alive": "1m"},
	}
	if after != nil {
		body["search_after"] = after
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return SearchResult{}, nil, fmt.Errorf("marshal search_after body: %w", err)
	}
	resp, err := s.client.Search().Raw(bytes.NewReader(raw)).TypedKeys(true).Do(ctx)
	if err != nil {
		return SearchResult{}, nil, err
	}
	res, err := toSearchResult(resp)
	if err != nil {
		return SearchResult{}, nil, err
	}
	var next []any
	if n := len(res.Hits); n > 0 {
		next = res.Hits[n-1].Sort
	}
	return res, next, nil
}

// ── 高级②：scroll 滚动遍历 ───────────────────────────────

// ScrollAll 用 scroll 遍历整个索引（导出场景），每批 batch 条，keepAlive 如 "1m"。
// scroll 在新版被 search_after+PIT 取代，这里演示经典用法：首个 Search 带 scroll 拿
// scroll_id，之后反复 Scroll 续拉，最后 ClearScroll 释放上下文。
func (s *DocStore) ScrollAll(ctx context.Context, batch int, keepAlive string) ([]Doc, error) {
	body, err := json.Marshal(map[string]any{
		"size":  batch,
		"query": map[string]any{"match_all": map[string]any{}},
		"sort":  []any{map[string]any{"id": map[string]any{"order": "asc"}}},
	})
	if err != nil {
		return nil, fmt.Errorf("marshal scroll body: %w", err)
	}
	resp, err := s.client.Search().Index(s.index).Scroll(keepAlive).Raw(bytes.NewReader(body)).Do(ctx)
	if err != nil {
		return nil, err
	}

	scrollId := ""
	if resp.ScrollId_ != nil {
		scrollId = *resp.ScrollId_
	}
	docs, err := hitsToDocs(resp.Hits.Hits)
	if err != nil {
		return nil, err
	}
	got := len(resp.Hits.Hits)

	for got == batch {
		cbody, mErr := json.Marshal(map[string]any{"scroll": keepAlive, "scroll_id": scrollId})
		if mErr != nil {
			err = fmt.Errorf("marshal scroll cont body: %w", mErr)
			break
		}
		sr, scErr := s.client.Scroll().Raw(bytes.NewReader(cbody)).Do(ctx)
		if scErr != nil {
			err = scErr
			break
		}
		if sr.ScrollId_ != nil {
			scrollId = *sr.ScrollId_
		}
		batchDocs, dErr := hitsToDocs(sr.Hits.Hits)
		if dErr != nil {
			err = dErr
			break
		}
		docs = append(docs, batchDocs...)
		got = len(sr.Hits.Hits)
	}

	// 清理 scroll 上下文；用独立 ctx 保证即使原 ctx 取消也执行，且不吞清理错误。
	if scrollId != "" {
		if _, clErr := s.client.ClearScroll().ScrollId(scrollId).Do(context.Background()); clErr != nil && err == nil {
			err = fmt.Errorf("clear scroll: %w", clErr)
		}
	}
	return docs, err
}

// ── 高级③：mget 批量按 id 取 ─────────────────────────────

// MGet 按多个 id 一次取回（一次 RTT），保持传入顺序，未命中的 id 跳过。
func (s *DocStore) MGet(ctx context.Context, ids []int64) ([]Doc, error) {
	strIds := make([]string, 0, len(ids))
	for _, id := range ids {
		strIds = append(strIds, docId(id))
	}
	resp, err := s.client.Mget().Index(s.index).Ids(strIds...).Do(ctx)
	if err != nil {
		return nil, err
	}
	docs := make([]Doc, 0, len(resp.Docs))
	for _, item := range resp.Docs {
		gr, ok := item.(*types.GetResult)
		if !ok || !gr.Found {
			continue // 未命中或错误项跳过
		}
		d, dErr := decodeDoc(gr.Source_)
		if dErr != nil {
			return nil, dErr
		}
		docs = append(docs, d)
	}
	return docs, nil
}
