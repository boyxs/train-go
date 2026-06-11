package source

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/webook/pkg/logger"
)

// newTestESClient 起 httptest 假 ES，elasticsearch.Client.Addresses 指向它。
// handler 接 *http.Request 让用例自定义响应；测试结束自动关闭 server。
func newTestESClient(t *testing.T, handler http.HandlerFunc) (*elasticsearch.Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	cli, err := elasticsearch.NewClient(elasticsearch.Config{
		Addresses: []string{srv.URL},
	})
	require.NoError(t, err)
	return cli, srv
}

// readBody helper：读 *http.Request.Body 转 string，便于断言请求体。
func readBody(t *testing.T, r *http.Request) string {
	t.Helper()
	b, err := io.ReadAll(r.Body)
	require.NoError(t, err)
	return string(b)
}

// stubSearchResponse 一段 ES _search hits 响应。
func stubSearchResponse(t *testing.T, hits []map[string]any) string {
	t.Helper()
	body := map[string]any{
		"hits": map[string]any{
			"total": map[string]any{"value": len(hits), "relation": "eq"},
			"hits":  hits,
		},
	}
	b, err := json.Marshal(body)
	require.NoError(t, err)
	return string(b)
}

func TestESSource_FullScan(t *testing.T) {
	t.Run("两批拉完(第二批小于 BatchSz 停止)", func(t *testing.T) {
		var callCount int
		cli, _ := newTestESClient(t, func(w http.ResponseWriter, r *http.Request) {
			callCount++
			// _search?index=article
			assert.True(t, strings.HasPrefix(r.URL.Path, "/article_v1/_search"), "path=%s", r.URL.Path)
			body := readBody(t, r)
			assert.Contains(t, body, `"search_after"`)
			assert.Contains(t, body, `"sort"`)

			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Elastic-Product", "Elasticsearch")
			switch callCount {
			case 1:
				_, _ = w.Write([]byte(stubSearchResponse(t, []map[string]any{
					{"_id": "1", "_source": map[string]any{"id": 1, "title": "a"}},
					{"_id": "2", "_source": map[string]any{"id": 2, "title": "b"}},
				})))
			case 2:
				_, _ = w.Write([]byte(stubSearchResponse(t, []map[string]any{
					{"_id": "3", "_source": map[string]any{"id": 3, "title": "c"}},
				})))
			default:
				t.Fatalf("unexpected call %d", callCount)
			}
		})

		s := NewESSource(cli, "article_v1", "id", logger.NewNopLogger())
		out := make(chan Row, 10)
		shard := ShardSpec{No: 0, PKMin: 1, PKMax: 100, BatchSz: 2}

		errCh := make(chan error, 1)
		go func() {
			defer close(out)
			errCh <- s.FullScan(context.Background(), shard, out)
		}()

		var rows []Row
		for r := range out {
			rows = append(rows, r)
		}
		require.NoError(t, <-errCh)
		require.Len(t, rows, 3)
		assert.Equal(t, "1", rows[0].PK)
		assert.Equal(t, "a", rows[0].Cols["title"])
		assert.Equal(t, "3", rows[2].PK)
		assert.Equal(t, 2, callCount)
	})

	t.Run("索引不存在 → ErrESIndexNotFound", func(t *testing.T) {
		cli, _ := newTestESClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Elastic-Product", "Elasticsearch")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"type":"index_not_found_exception"}}`))
		})

		s := NewESSource(cli, "missing_index", "id", logger.NewNopLogger())
		out := make(chan Row, 1)
		err := s.FullScan(context.Background(), ShardSpec{PKMax: 100, BatchSz: 10}, out)
		assert.ErrorIs(t, err, ErrESIndexNotFound)
	})

	t.Run("Cols.id 缺失时 fallback _id 转 int64", func(t *testing.T) {
		cli, _ := newTestESClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Elastic-Product", "Elasticsearch")
			_, _ = w.Write([]byte(stubSearchResponse(t, []map[string]any{
				// _source 里没有 id 字段，只能从 _id 拿
				{"_id": "42", "_source": map[string]any{"title": "z"}},
			})))
		})
		s := NewESSource(cli, "x", "id", logger.NewNopLogger())
		out := make(chan Row, 1)
		errCh := make(chan error, 1)
		go func() {
			defer close(out)
			errCh <- s.FullScan(context.Background(), ShardSpec{PKMax: 100, BatchSz: 10}, out)
		}()
		var got []Row
		for r := range out {
			got = append(got, r)
		}
		require.NoError(t, <-errCh)
		require.Len(t, got, 1)
		assert.Equal(t, "42", got[0].PK)
	})
}

func TestESSource_PKRange(t *testing.T) {
	t.Run("正常返回 min/max", func(t *testing.T) {
		cli, _ := newTestESClient(t, func(w http.ResponseWriter, r *http.Request) {
			body := readBody(t, r)
			assert.Contains(t, body, `"aggs"`)
			assert.Contains(t, body, `"min_pk"`)
			assert.Contains(t, body, `"max_pk"`)

			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Elastic-Product", "Elasticsearch")
			_, _ = w.Write([]byte(`{
				"aggregations":{
					"min_pk":{"value":1},
					"max_pk":{"value":100}
				}
			}`))
		})
		s := NewESSource(cli, "x", "id", logger.NewNopLogger())
		min, max, err := s.(*ESSource).PKRange(context.Background())
		require.NoError(t, err)
		assert.Equal(t, int64(1), min)
		assert.Equal(t, int64(100), max)
	})

	t.Run("空索引返 (0, 0, nil)", func(t *testing.T) {
		cli, _ := newTestESClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Elastic-Product", "Elasticsearch")
			_, _ = w.Write([]byte(`{
				"aggregations":{
					"min_pk":{"value":null},
					"max_pk":{"value":null}
				}
			}`))
		})
		s := NewESSource(cli, "x", "id", logger.NewNopLogger())
		min, max, err := s.(*ESSource).PKRange(context.Background())
		require.NoError(t, err)
		assert.Equal(t, int64(0), min)
		assert.Equal(t, int64(0), max)
	})
}
