package es

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/elastic/go-elasticsearch/v9/typedapi/types/enums/refresh"
)

// ── C 批量 Bulk ──────────────────────────────────────────

// BulkIndex 批量写入一组文档（最常见用法）。
func (s *DocStore) BulkIndex(ctx context.Context, docs []Doc) (BulkStats, error) {
	actions := make([]BulkAction, 0, len(docs))
	for _, d := range docs {
		actions = append(actions, BulkAction{Op: "index", Id: d.Id, Doc: d})
	}
	return s.Bulk(ctx, actions)
}

// Bulk 执行混合批量操作，手工拼 NDJSON（每条 = 元数据行 [+ 文档行]，delete 无文档行），
// 直观呈现 bulk 协议。逐条读 items 统计成功/失败——errors=true 时也不整体报错，交由调用方
// 按 Failures 处理（这正是 bulk 的部分失败语义）。
func (s *DocStore) Bulk(ctx context.Context, actions []BulkAction) (BulkStats, error) {
	var buf bytes.Buffer
	for _, a := range actions {
		meta := map[string]any{a.Op: map[string]any{"_id": docId(a.Id)}}
		metaLine, err := json.Marshal(meta)
		if err != nil {
			return BulkStats{}, fmt.Errorf("marshal bulk meta: %w", err)
		}
		buf.Write(metaLine)
		buf.WriteByte('\n')
		if a.Op == "delete" {
			continue
		}
		payload := a.Doc
		if a.Op == "update" {
			payload = map[string]any{"doc": a.Doc}
		}
		docLine, err := json.Marshal(payload)
		if err != nil {
			return BulkStats{}, fmt.Errorf("marshal bulk doc: %w", err)
		}
		buf.Write(docLine)
		buf.WriteByte('\n')
	}

	resp, err := s.client.Bulk().Index(s.index).Raw(bytes.NewReader(buf.Bytes())).Refresh(refresh.True).Do(ctx)
	if err != nil {
		return BulkStats{}, err
	}

	stats := BulkStats{Total: len(resp.Items)}
	for _, item := range resp.Items {
		for op, res := range item {
			if res.Status >= http.StatusOK && res.Status < http.StatusMultipleChoices {
				stats.Succeeded++
				continue
			}
			stats.Failed++
			f := BulkFailure{Op: op.Name, Status: res.Status}
			if res.Id_ != nil {
				if v, perr := strconv.ParseInt(*res.Id_, 10, 64); perr == nil {
					f.Id = v
				}
			}
			if res.Error != nil {
				f.Reason = res.Error.Type
			}
			stats.Failures = append(stats.Failures, f)
		}
	}
	return stats, nil
}
