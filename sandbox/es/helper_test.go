package es

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// 集成测试：连真实 ES（默认 127.0.0.1:9200，可用 ES_ADDR 覆盖）。每个测试用**各自独占**的
// 索引（名字由测试名生成，先删后建），teardown 自动删除，互不干扰、并行安全。
//
// 运行：cd sandbox/es && go test -v ./...
//       ES_ADDR=http://host:9200 go test -v ./...

// indexName 按测试名生成独占索引名（ES 索引名须小写、不含特殊字符）。每个测试各用一个索引，
// 避免所有测试共享一个名字反复删建带来的偶发竞态（删/建/写紧挨着跑，索引重建有传播窗口，
// bulk 写会偶发 index_not_found）。
func indexName(t *testing.T) string {
	name := strings.ToLower(t.Name())
	name = strings.NewReplacer("/", "_", " ", "_").Replace(name)
	return "es_demo_" + name
}

// newStore 建一个绑定「本测试独占索引」的 DocStore。连本地 ES，凭据默认 elastic/elastic
// （本地 xpack.security 开启后的密码），可用 ES_ADDR / ES_USER / ES_PASS 覆盖。
func newStore(t *testing.T) *DocStore {
	t.Helper()
	client, err := NewClient(os.Getenv("ES_ADDR"), envOr("ES_USER", "elastic"), envOr("ES_PASS", "elastic"))
	require.NoError(t, err, "创建 ES 客户端")
	return NewDocStore(client, indexName(t))
}

// envOr 取环境变量，为空则回落默认值。
func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// freshStore 建一个全新的空索引（先删后建），并注册 teardown 删除。
func freshStore(t *testing.T) *DocStore {
	t.Helper()
	s := newStore(t)
	ctx := context.Background()
	require.NoError(t, s.DeleteIndex(ctx), "前置清理：删索引")
	require.NoError(t, s.CreateIndex(ctx), "建索引")
	t.Cleanup(func() {
		if err := s.DeleteIndex(context.Background()); err != nil {
			t.Errorf("teardown 删索引失败: %v", err)
		}
	})
	return s
}

// seedStore 建索引 + 批量灌入标准样本数据，供搜索/聚合/计数/高级类测试复用。
func seedStore(t *testing.T) *DocStore {
	t.Helper()
	s := freshStore(t)
	stats, err := s.BulkIndex(context.Background(), sampleDocs())
	require.NoError(t, err, "灌样本数据")
	require.Zero(t, stats.Failed, "样本数据应全部写入成功: %+v", stats.Failures)
	return s
}

// sampleDocs 固定样本：content 用空格分词英文，标准分词器下 match 命中确定；category/tags
// 为 keyword 精确匹配；score/views/created_at 便于范围/排序/聚合断言。
func sampleDocs() []Doc {
	const base = int64(1_700_000_000_000) // 固定基准毫秒时间戳
	const day = int64(24 * 60 * 60 * 1000)
	return []Doc{
		{Id: 1, Title: "无线蓝牙耳机", Category: "electronics", Tags: []string{"audio", "wireless"}, Score: 199, Views: 50, Content: "wireless bluetooth headphone with noise cancelling", CreatedAt: base},
		{Id: 2, Title: "机械键盘", Category: "electronics", Tags: []string{"input", "rgb"}, Score: 399, Views: 30, Content: "mechanical keyboard with rgb backlight", CreatedAt: base + day},
		{Id: 3, Title: "人体工学椅", Category: "furniture", Tags: []string{"office", "ergonomic"}, Score: 1299, Views: 10, Content: "ergonomic office chair with lumbar support", CreatedAt: base + 2*day},
		{Id: 4, Title: "机械表", Category: "watch", Tags: []string{"luxury", "mechanical"}, Score: 8999, Views: 5, Content: "automatic mechanical wristwatch", CreatedAt: base + 3*day},
		{Id: 5, Title: "蓝牙音箱", Category: "electronics", Tags: []string{"audio", "wireless"}, Score: 299, Views: 40, Content: "portable wireless bluetooth speaker waterproof", CreatedAt: base + 4*day},
		{Id: 6, Title: "办公桌", Category: "furniture", Tags: []string{"office"}, Score: 799, Views: 15, Content: "height adjustable office desk", CreatedAt: base + 5*day},
	}
}

// mappingFieldType 从 GetMapping 的原始响应里取某字段的 ES type（idx=索引名）。
func mappingFieldType(t *testing.T, m map[string]any, idx, field string) string {
	t.Helper()
	rec, ok := m[idx].(map[string]any)
	require.True(t, ok, "响应缺 %s: %v", idx, m)
	mappings, ok := rec["mappings"].(map[string]any)
	require.True(t, ok, "缺 mappings")
	props, ok := mappings["properties"].(map[string]any)
	require.True(t, ok, "缺 properties")
	f, ok := props[field].(map[string]any)
	require.True(t, ok, "缺字段 %s", field)
	typ, ok := f["type"].(string)
	require.True(t, ok, "字段 %s 缺 type", field)
	return typ
}

// hitIds 按命中顺序取文档 Id（排序/分页断言用）。
func hitIds(r SearchResult) []int64 {
	out := make([]int64, 0, len(r.Hits))
	for _, h := range r.Hits {
		out = append(out, h.Doc.Id)
	}
	return out
}

// docIds 从 Doc 切片取 Id（scroll/mget 断言用）。
func docIds(ds []Doc) []int64 {
	out := make([]int64, 0, len(ds))
	for _, d := range ds {
		out = append(out, d.Id)
	}
	return out
}
