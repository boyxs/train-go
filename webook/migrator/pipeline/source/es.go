package source

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"

	"github.com/webook/pkg/logger"
)

// ESSource 用于异构对账场景下读 ES 端数据，实现 FullSource。
//
//   - FullScan 用 search_after 分页扫所有 doc（替代 ES 8 已 deprecated 的 scroll）
//   - PKRange 用 aggs min/max 一次 RTT 拿范围（PKRanger 接口）
//   - 增量请用源端 MySQL Canal（ES 无 binlog 概念）
//   - Close no-op
//
// PK 字段名来自 TableMapping.PartitionKey（默认 "id"），要求 ES mapping 中该字段是数值类型。
// _id 也优先用 _source[pkField] 转 int64；解不到 fallback _id 字符串转 int64。
//
// 测试策略：用 httptest.NewServer 起假 ES，elasticsearch.NewClient({Addresses: server.URL}) 让请求真打过去。
type ESSource struct {
	client  *elasticsearch.Client
	index   string
	pkField string // 默认 "id"
	l       logger.LoggerX
}

// ErrESIndexNotFound ES 索引不存在 — 比 raw 404 友好。
var ErrESIndexNotFound = errors.New("es index not found")

func NewESSource(client *elasticsearch.Client, indexName, pkField string, l logger.LoggerX) FullSource {
	if pkField == "" {
		pkField = "id"
	}
	return &ESSource{client: client, index: indexName, pkField: pkField, l: l}
}

// FullScan 用 search_after 分页拉所有 doc，按 PK 升序。
//
// 与 MySQLSource.FullScan 语义一致：
//   - shard.PKMin/PKMax 用作 range 过滤；shard.BatchSz 控制单批 size
//   - 每条 doc 转 Row{Table:index, PK, Cols:_source} 推 out
//   - 本批返回 < BatchSz 表示扫完，退出
//
// 限速（shard.QPSLimit）：MVP 不实现 — ES search 通常受集群限速保护；
// 若需限速可在工厂层包装 ratelimit decorator。
func (s *ESSource) FullScan(ctx context.Context, shard ShardSpec, out chan<- Row) error {
	batch := shard.BatchSz
	if batch <= 0 {
		batch = defaultBatchSz
	}
	// search_after 起点：用 shard.PKMin - 1（首批 _gt PKMin-1 等价 _gte PKMin）
	lastPK := shard.PKMin - 1
	for {
		body := map[string]any{
			"size": batch,
			"sort": []any{
				map[string]any{s.pkField: map[string]any{"order": "asc"}},
			},
			"search_after": []any{lastPK},
			"query": map[string]any{
				"range": map[string]any{
					s.pkField: map[string]any{
						"lte": shard.PKMax,
					},
				},
			},
		}
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal es search body: %w", err)
		}
		res, err := s.client.Search(
			s.client.Search.WithContext(ctx),
			s.client.Search.WithIndex(s.index),
			s.client.Search.WithBody(bytes.NewReader(buf)),
		)
		if err != nil {
			return fmt.Errorf("es search shard %d: %w", shard.No, err)
		}
		hits, scanErr := s.consumeSearchResponse(res)
		if scanErr != nil {
			return fmt.Errorf("es search shard %d: %w", shard.No, scanErr)
		}
		if len(hits) == 0 {
			return nil
		}
		for _, h := range hits {
			pk := s.extractPK(h)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case out <- Row{Table: s.index, PK: strconv.FormatInt(pk, 10), Cols: h.Source}:
			}
			if pk > lastPK {
				lastPK = pk
			}
		}
		if len(hits) < batch {
			return nil
		}
	}
}

// PKRange 实现 PKRanger — 用 aggs 一次 RTT 拿 (min, max)。
// 空索引返 (0, 0, nil) — 调用方按 0 范围跳过 FullScan。
func (s *ESSource) PKRange(ctx context.Context) (int64, int64, error) {
	body := map[string]any{
		"size": 0,
		"aggs": map[string]any{
			"min_pk": map[string]any{"min": map[string]any{"field": s.pkField}},
			"max_pk": map[string]any{"max": map[string]any{"field": s.pkField}},
		},
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return 0, 0, fmt.Errorf("marshal pk range body: %w", err)
	}
	res, err := s.client.Search(
		s.client.Search.WithContext(ctx),
		s.client.Search.WithIndex(s.index),
		s.client.Search.WithBody(bytes.NewReader(buf)),
	)
	if err != nil {
		return 0, 0, fmt.Errorf("es pk range search: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		if res.StatusCode == 404 {
			return 0, 0, ErrESIndexNotFound
		}
		body, _ := io.ReadAll(res.Body)
		return 0, 0, fmt.Errorf("es pk range %d: %s", res.StatusCode, body)
	}
	var aggResp struct {
		Aggregations struct {
			MinPK struct {
				Value *float64 `json:"value"`
			} `json:"min_pk"`
			MaxPK struct {
				Value *float64 `json:"value"`
			} `json:"max_pk"`
		} `json:"aggregations"`
	}
	if err := json.NewDecoder(res.Body).Decode(&aggResp); err != nil {
		return 0, 0, fmt.Errorf("decode pk range: %w", err)
	}
	if aggResp.Aggregations.MinPK.Value == nil || aggResp.Aggregations.MaxPK.Value == nil {
		return 0, 0, nil // 空索引
	}
	return int64(*aggResp.Aggregations.MinPK.Value), int64(*aggResp.Aggregations.MaxPK.Value), nil
}

func (s *ESSource) Close() error {
	return nil
}

// esHit 单条 doc 反序列化结果。
type esHit struct {
	ID     string         `json:"_id"`
	Source map[string]any `json:"_source"`
}

// consumeSearchResponse 关闭 Response.Body + 解析出 hits 列表。
// 404 → ErrESIndexNotFound;其他 4xx/5xx → 通用 fmt.Errorf。
func (s *ESSource) consumeSearchResponse(res *esapi.Response) ([]esHit, error) {
	defer res.Body.Close()
	if res.IsError() {
		if res.StatusCode == 404 {
			return nil, ErrESIndexNotFound
		}
		body, _ := io.ReadAll(res.Body)
		return nil, fmt.Errorf("es search %d: %s", res.StatusCode, body)
	}
	var sr struct {
		Hits struct {
			Hits []esHit `json:"hits"`
		} `json:"hits"`
	}
	if err := json.NewDecoder(res.Body).Decode(&sr); err != nil {
		return nil, fmt.Errorf("decode search hits: %w", err)
	}
	return sr.Hits.Hits, nil
}

// extractPK 优先从 _source[pkField] 拿（保数值精度），fallback _id 字符串解析。
func (s *ESSource) extractPK(h esHit) int64 {
	if h.Source != nil {
		if v, ok := toInt64(h.Source[s.pkField]); ok {
			return v
		}
	}
	if v, err := strconv.ParseInt(h.ID, 10, 64); err == nil {
		return v
	}
	return 0
}
