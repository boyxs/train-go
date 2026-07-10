package es

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── 高级①：PIT + search_after 深分页 ─────────────────────

func TestAdvanced_SearchAfterPIT(t *testing.T) {
	s := seedStore(t)
	ctx := context.Background()

	pit, err := s.OpenPIT(ctx, "1m")
	require.NoError(t, err)
	require.NotEmpty(t, pit)
	t.Cleanup(func() {
		if err := s.ClosePIT(context.Background(), pit); err != nil {
			t.Errorf("close pit: %v", err)
		}
	})

	sort := []any{map[string]any{"id": map[string]any{"order": "asc"}}}
	var all []int64
	var after []any
	for {
		res, next, err := s.SearchAfter(ctx, pit, 2, sort, after)
		require.NoError(t, err)
		if len(res.Hits) == 0 {
			break
		}
		all = append(all, hitIds(res)...)
		after = next
	}
	assert.Equal(t, []int64{1, 2, 3, 4, 5, 6}, all, "search_after 应按 id 升序翻完全部")
}

// ── 高级②：scroll 滚动遍历 ───────────────────────────────

func TestAdvanced_Scroll(t *testing.T) {
	s := seedStore(t)
	docs, err := s.ScrollAll(context.Background(), 2, "1m")
	require.NoError(t, err)
	assert.Equal(t, []int64{1, 2, 3, 4, 5, 6}, docIds(docs), "scroll 应遍历全部（id 升序）")
}

// ── 高级③：mget 批量按 id 取 ─────────────────────────────

func TestAdvanced_MGet(t *testing.T) {
	s := seedStore(t)
	docs, err := s.MGet(context.Background(), []int64{2, 4, 6, 999})
	require.NoError(t, err)
	assert.Equal(t, []int64{2, 4, 6}, docIds(docs), "按 id 取回，缺失(999)跳过")
}

// ── 高级④：模糊查询族 ────────────────────────────────────

func TestAdvanced_Fuzzy(t *testing.T) {
	s := seedStore(t)
	// content 里 "wireless" 拼错成 "wireles"，fuzzy 容 1 编辑距离仍命中 doc1、doc5
	r, err := s.Search(context.Background(), map[string]any{
		"query": map[string]any{"fuzzy": map[string]any{"content": map[string]any{"value": "wireles", "fuzziness": 1}}},
	})
	require.NoError(t, err)
	assert.ElementsMatch(t, []int64{1, 5}, hitIds(r))
}

func TestAdvanced_Wildcard(t *testing.T) {
	s := seedStore(t)
	// category 是 keyword，wildcard 匹配整个 term；electro* → electronics
	r, err := s.Search(context.Background(), map[string]any{
		"query": map[string]any{"wildcard": map[string]any{"category": map[string]any{"value": "electro*"}}},
	})
	require.NoError(t, err)
	assert.ElementsMatch(t, []int64{1, 2, 5}, hitIds(r))
}

func TestAdvanced_Prefix(t *testing.T) {
	s := seedStore(t)
	// category 前缀 "fur" → furniture
	r, err := s.Search(context.Background(), map[string]any{
		"query": map[string]any{"prefix": map[string]any{"category": "fur"}},
	})
	require.NoError(t, err)
	assert.ElementsMatch(t, []int64{3, 6}, hitIds(r))
}

func TestAdvanced_MultiMatch(t *testing.T) {
	s := seedStore(t)
	// "office" 跨 title + content 检索 → content 含 office 的 doc3、doc6
	r, err := s.Search(context.Background(), map[string]any{
		"query": map[string]any{"multi_match": map[string]any{"query": "office", "fields": []string{"title", "content"}}},
	})
	require.NoError(t, err)
	assert.ElementsMatch(t, []int64{3, 6}, hitIds(r))
}

// ── 高级⑤：脚本打分 + 折叠 ───────────────────────────────

func TestAdvanced_FunctionScore(t *testing.T) {
	s := seedStore(t)
	// function_score：用 views 加权（field_value_factor），boost_mode=replace → score=views；
	// views 最高的 doc1(50) 排第一
	r, err := s.Search(context.Background(), map[string]any{
		"query": map[string]any{
			"function_score": map[string]any{
				"query":              map[string]any{"match_all": map[string]any{}},
				"field_value_factor": map[string]any{"field": "views"},
				"boost_mode":         "replace",
			},
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, r.Hits)
	assert.Equal(t, int64(1), r.Hits[0].Doc.Id, "views 最高(50)的 doc1 应排第一")
}

func TestAdvanced_ScriptScore(t *testing.T) {
	s := seedStore(t)
	// script_score：直接用 score 字段值作为相关性得分（项目 kNN 用同款 script_score 模式）；
	// score 最高的 doc4(8999) 排第一
	r, err := s.Search(context.Background(), map[string]any{
		"query": map[string]any{
			"script_score": map[string]any{
				"query":  map[string]any{"match_all": map[string]any{}},
				"script": map[string]any{"source": "doc['score'].value"},
			},
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, r.Hits)
	assert.Equal(t, int64(4), r.Hits[0].Doc.Id, "score 最高(8999)的 doc4 应排第一")
}

func TestAdvanced_Collapse(t *testing.T) {
	s := seedStore(t)
	// collapse 按 category 折叠，每个 category 只留 1 条 → 3 个 category = 3 条命中
	r, err := s.Search(context.Background(), map[string]any{
		"collapse": map[string]any{"field": "category"},
		"query":    map[string]any{"match_all": map[string]any{}},
	})
	require.NoError(t, err)
	assert.Len(t, r.Hits, 3, "3 个 category 折叠后 3 条")
	cats := map[string]bool{}
	for _, h := range r.Hits {
		cats[h.Doc.Category] = true
	}
	assert.Len(t, cats, 3, "折叠后 category 互不相同")
}
