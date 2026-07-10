package es

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// ── A 索引管理 ───────────────────────────────────────────

// CreateIndex 用 doc_index.json 的 mapping 建索引。
func (s *DocStore) CreateIndex(ctx context.Context) error {
	_, err := s.client.Indices.Create(s.index).Raw(bytes.NewReader(indexMappingJSON)).Do(ctx)
	return err
}

// IndexExists 判断索引是否存在。
func (s *DocStore) IndexExists(ctx context.Context) (bool, error) {
	return s.client.Indices.Exists(s.index).Do(ctx)
}

// GetMapping 读回索引 mapping 原始 JSON（用 Perform 拿原始响应体，便于断言字段类型）。
func (s *DocStore) GetMapping(ctx context.Context) (map[string]any, error) {
	resp, err := s.client.Indices.GetMapping().Index(s.index).Perform(ctx)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("get mapping: 非预期状态码 %d", resp.StatusCode)
	}
	var out map[string]any
	if err = json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode mapping: %w", err)
	}
	return out, nil
}

// DeleteIndex 删除索引；索引不存在（404）视为已删返回 nil（幂等，便于前置清理与 teardown 复用）。
func (s *DocStore) DeleteIndex(ctx context.Context) error {
	_, err := s.client.Indices.Delete(s.index).Do(ctx)
	if err != nil && !IsNotFound(err) {
		return err
	}
	return nil
}
