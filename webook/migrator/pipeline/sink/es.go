package sink

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/elastic/go-elasticsearch/v9"

	"github.com/boyxs/train-go/webook/pkg/logger"
)

// ESSink 把 Mutation batch 写入 Elasticsearch（bulk API）。
//
// 设计：
//   - 索引名 = indexName（构造时传入），文档 id = Mutation.PK
//   - Op：insert/update → index（幂等覆盖）；delete → delete
//   - Bulk request 失败 → 返 error；单条 item 失败 → log warn 但整批继续
//
// Version 乐观锁通过 ES external version 实现：
//
//	"version_type": "external", "version": Mutation.Version
//
// 同 ID 旧 version 写入会被 ES 拒（409 conflict），保证「老 binlog 不覆盖新值」。
type ESSink struct {
	client    *elasticsearch.Client
	indexName string
	l         logger.LoggerX
}

func NewESSink(client *elasticsearch.Client, indexName string, l logger.LoggerX) Sink {
	return &ESSink{client: client, indexName: indexName, l: l}
}

func (s *ESSink) Apply(ctx context.Context, batch []Mutation) error {
	if len(batch) == 0 {
		return nil
	}
	var buf bytes.Buffer
	for _, m := range batch {
		if err := writeESAction(&buf, s.indexName, m); err != nil {
			return fmt.Errorf("encode bulk action: %w", err)
		}
	}
	res, err := s.client.Bulk(bytes.NewReader(buf.Bytes()), s.client.Bulk.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("es bulk: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		body, rerr := io.ReadAll(res.Body)
		if rerr != nil {
			s.l.Warn("read es error body failed",
				logger.Int("status", res.StatusCode), logger.Error(rerr))
		}
		return fmt.Errorf("es bulk %d: %s", res.StatusCode, body)
	}
	// 解析 bulk response 统计 item 级失败（version conflict 算业务正常 → 不上抛）
	var br bulkResponse
	if err := json.NewDecoder(res.Body).Decode(&br); err != nil {
		return fmt.Errorf("decode bulk response: %w", err)
	}
	if br.Errors {
		for _, item := range br.Items {
			for op, detail := range item {
				if detail.Status >= 400 && detail.Status != http.StatusConflict {
					s.l.Warn("es bulk item failed",
						logger.String("op", op),
						logger.Int("status", detail.Status),
						logger.String("error", detail.Error.Reason))
				}
			}
		}
	}
	return nil
}

func (s *ESSink) Close() error { return nil }

type bulkResponse struct {
	Errors bool                                `json:"errors"`
	Items  []map[string]bulkResponseItemDetail `json:"items"`
}

type bulkResponseItemDetail struct {
	Status int                 `json:"status"`
	Error  bulkResponseItemErr `json:"error,omitempty"`
}

type bulkResponseItemErr struct {
	Type   string `json:"type"`
	Reason string `json:"reason"`
}

// writeESAction 按 Mutation.Op 生成 bulk action + doc 行。
// External version 控制：Version > 0 时启用乐观锁。
func writeESAction(buf *bytes.Buffer, index string, m Mutation) error {
	docID := m.PK
	switch m.Op {
	case OpInsert, OpUpdate:
		action := map[string]any{
			"index": map[string]any{
				"_index": index,
				"_id":    docID,
			},
		}
		if m.Version > 0 {
			action["index"].(map[string]any)["version"] = m.Version
			action["index"].(map[string]any)["version_type"] = "external"
		}
		ab, err := json.Marshal(action)
		if err != nil {
			return fmt.Errorf("marshal es index action pk=%s: %w", m.PK, err)
		}
		buf.Write(ab)
		buf.WriteByte('\n')
		db, err := json.Marshal(m.Cols)
		if err != nil {
			return fmt.Errorf("marshal es doc pk=%s: %w", m.PK, err)
		}
		buf.Write(db)
		buf.WriteByte('\n')
	case OpDelete:
		action := map[string]any{
			"delete": map[string]any{
				"_index": index,
				"_id":    docID,
			},
		}
		ab, err := json.Marshal(action)
		if err != nil {
			return fmt.Errorf("marshal es delete action pk=%s: %w", m.PK, err)
		}
		buf.Write(ab)
		buf.WriteByte('\n')
	default:
		return fmt.Errorf("unknown op %q", m.Op)
	}
	return nil
}
