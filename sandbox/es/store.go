// Package es 是 go-elasticsearch/v9 TypedClient 的全面用法示范，对齐真实 ES 9.x。
//
// 交付形态：由集成测试驱动的薄封装 DocStore，无 main。方法按能力分散在各文件：
//
//	doc.go          文档实体 Doc + mapping
//	store.go        DocStore + 客户端工厂 + 共享结果类型 + 错误判定
//	index.go        索引管理（建/exists/读mapping/删）
//	document.go     文档 CRUD（index/create/get/update/delete/exists）
//	bulk.go         批量 Bulk（index/混合/部分失败）
//	search.go       搜索 + 计数（match/term/bool/range/分页/排序/高亮/_source/agg 入口）
//	aggregation.go  聚合的强类型解析封装（terms/stats）
//	advanced.go     高级：PIT + search_after 深分页、scroll 遍历、mget 批量取
//
// 查询 / 映射沿用项目 internal/repository/dao/article_search.go 的 map[string]any +
// Raw(bytes.Reader) 风格，直观呈现 ES DSL；响应统一走 TypedClient 强类型结构解析。
package es

import (
	"errors"
	"net/http"

	"github.com/elastic/go-elasticsearch/v9"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types"
)

// DefaultIndex demo 专用索引名，测试建了即删，不碰其它数据。
const DefaultIndex = "es_demo_doc"

// DocStore 封装 TypedClient，绑定单个索引。构造函数返回具体指针（sandbox demo，
// 非分层业务代码，无需接口抽象）。
type DocStore struct {
	client *elasticsearch.TypedClient
	index  string
}

// NewDocStore 绑定 client + 索引名。
func NewDocStore(client *elasticsearch.TypedClient, index string) *DocStore {
	return &DocStore{client: client, index: index}
}

// NewClient 按地址建 TypedClient；addr 为空默认连本地 127.0.0.1:9200。
// username/password 为空则不认证；连启用 xpack.security 的 ES 时填 Basic 认证凭据。
// v9 用函数式 Option（NewTypedClient/Config 已弃用）。
func NewClient(addr, username, password string) (*elasticsearch.TypedClient, error) {
	if addr == "" {
		addr = "http://127.0.0.1:9200"
	}
	opts := []elasticsearch.Option{elasticsearch.WithAddresses(addr)}
	if username != "" || password != "" {
		opts = append(opts, elasticsearch.WithBasicAuth(username, password))
	}
	return elasticsearch.NewTyped(opts...)
}

// ── 共享结果类型 ─────────────────────────────────────────

// SearchHit 单条命中：文档 + 高亮片段 + 排序值 + 相关性得分。
type SearchHit struct {
	Doc       Doc
	Highlight map[string][]string
	Sort      []any
	Score     float64
}

// SearchResult 搜索结果：总命中数 + 命中列表 + 聚合结果（强类型 union，按需断言）。
type SearchResult struct {
	Total int64
	Hits  []SearchHit
	Aggs  map[string]types.Aggregate
}

// Docs 便捷取出命中的文档列表。
func (r SearchResult) Docs() []Doc {
	ds := make([]Doc, 0, len(r.Hits))
	for _, h := range r.Hits {
		ds = append(ds, h.Doc)
	}
	return ds
}

// BulkAction 一条 bulk 操作。Op ∈ index/create/update/delete；
// update 的 Doc 是部分字段 map，index/create 的 Doc 是 Doc，delete 无 Doc。
type BulkAction struct {
	Op  string
	Id  int64
	Doc any
}

// BulkFailure 单条失败操作。
type BulkFailure struct {
	Id     int64
	Op     string
	Status int
	Reason string
}

// BulkStats 批量结果统计。
type BulkStats struct {
	Total     int
	Succeeded int
	Failed    int
	Failures  []BulkFailure
}

// hitsToDocs 把命中列表解析成 Doc 切片（scroll/遍历类复用）。
func hitsToDocs(hits []types.Hit) ([]Doc, error) {
	docs := make([]Doc, 0, len(hits))
	for _, h := range hits {
		d, err := decodeDoc(h.Source_)
		if err != nil {
			return nil, err
		}
		docs = append(docs, d)
	}
	return docs, nil
}

// ── 错误判定 ─────────────────────────────────────────────

// IsConflict 判断 err 是否 ES 409（版本冲突 / 文档已存在）。
func IsConflict(err error) bool { return statusIs(err, http.StatusConflict) }

// IsNotFound 判断 err 是否 ES 404（索引 / 文档不存在）。
func IsNotFound(err error) bool { return statusIs(err, http.StatusNotFound) }

// statusIs 从 TypedClient 返回的错误里取 ES HTTP 状态码比对。
func statusIs(err error, code int) bool {
	var esErr *types.ElasticsearchError
	return errors.As(err, &esErr) && esErr.Status == code
}
