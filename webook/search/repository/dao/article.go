package dao

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"sync"

	"github.com/elastic/go-elasticsearch/v9"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types"

	"github.com/boyxs/train-go/webook/pkg/logger"
)

// ErrESDocNotFound 删除不存在的文档时返回（repo/service 层可映射为业务错误）。
var ErrESDocNotFound = errors.New("es doc not found")

//go:embed article_index.json
var articleIndexMapping []byte

// ArticleESDoc ES 文档结构。Category / Tags（keyword）供 facet 过滤/聚合。
type ArticleESDoc struct {
	Id         int64     `json:"id"`
	Title      string    `json:"title"`
	Abstract   string    `json:"abstract"`
	AuthorId   int64     `json:"author_id"`
	AuthorName string    `json:"author_name"`
	Status     uint8     `json:"status"`
	Category   string    `json:"category"`
	Tags       []string  `json:"tags"`
	CreatedAt  int64     `json:"created_at"`
	ContentVec []float32 `json:"content_vec"`
}

// TagCount 标签聚合计数（facet / 推荐候选）。key 是 tag slug（ES tags 存 slug）。
type TagCount struct {
	Slug  string
	Count int64
}

type ArticleDAO interface {
	Upsert(ctx context.Context, doc ArticleESDoc) error
	Delete(ctx context.Context, id int64) error
	// Search 混合检索（BM25 + 向量）+ 标签 facet：
	// filterTags 非空则 post_filter 按标签 AND 过滤 hits；facets 恒为关键词命中集的标签 terms 聚合（不受 post_filter 影响）。
	Search(ctx context.Context, text string, vec []float32, filterTags []string, offset, limit int) (docs []ArticleESDoc, total int64, facets []TagCount, err error)
	// RecommendTags kNN 取 topK 相似（已发布）文章，聚合其标签作候选（复用 content_vec，无需 LLM）。
	RecommendTags(ctx context.Context, vec []float32, k int) ([]TagCount, error)
}

type ElasticArticleDAO struct {
	client   *elasticsearch.TypedClient
	index    string
	l        logger.LoggerX
	ensureMu sync.Mutex
	ensured  bool // 别名→物理索引是否已确保就绪（写路径 fail-fast 依据）
}

func NewElasticArticleDAO(client *elasticsearch.TypedClient, index string, l logger.LoggerX) ArticleDAO {
	d := &ElasticArticleDAO{client: client, index: index, l: l}
	if err := d.ensureIndex(); err != nil {
		// 启动仅告警不阻断（依赖不可达即降级，与本服务风格一致）；写路径会懒重试并 fail-fast。
		l.Warn(context.Background(), "启动 ensureIndex 失败，写路径将懒重试确保", logger.Error(err))
	}
	return d
}

// IndexVersionSuffix 物理索引的版本后缀。app 与查询只认稳定的逻辑「别名」（d.index，如 "article"），
// 物理索引是版本化的 "<alias>_v1"。改 mapping 时新建 "<alias>_v2" 重灌数据、再原子切别名 → 零停机 reindex。
const IndexVersionSuffix = "_v1"

// ensureIndex 幂等确保「别名 d.index → 物理版本索引 <d.index>_v1」存在：
//   - 别名已存在 → 完成；
//   - 物理索引不存在 → 按 article_index.json 建；
//   - 把别名绑到物理索引（存量部署只有物理索引、无别名时，本步补挂别名、数据不动，平滑迁移）。
//
// 成功置 d.ensured=true；失败返回 error（启动仅告警、写路径 fail-fast）。写/查全程走别名——见 Upsert/Delete/Search/RecommendTags 用 d.index。
func (d *ElasticArticleDAO) ensureIndex() error {
	ctx := context.Background()
	alias := d.index
	physical := alias + IndexVersionSuffix

	aliasExists, err := d.client.Indices.ExistsAlias(alias).Do(ctx)
	if err != nil {
		return fmt.Errorf("检查 ES 别名失败: %w", err)
	}
	if aliasExists {
		d.ensured = true
		return nil
	}
	physExists, err := d.client.Indices.Exists(physical).Do(ctx)
	if err != nil {
		return fmt.Errorf("检查 ES 索引失败: %w", err)
	}
	if !physExists {
		if _, err = d.client.Indices.Create(physical).Raw(bytes.NewReader(articleIndexMapping)).Do(ctx); err != nil {
			return fmt.Errorf("创建 ES 索引 %s 失败: %w", physical, err)
		}
	}
	if _, err = d.client.Indices.PutAlias(physical, alias).Do(ctx); err != nil {
		return fmt.Errorf("绑定 ES 别名 %s→%s 失败: %w", alias, physical, err)
	}
	d.ensured = true
	return nil
}

// ensureReady 写前确保别名→物理索引就绪（懒重试启动时失败的 ensureIndex）。
// 为什么写路径必须拦：ES auto_create_index 会用动态映射把首个写自动建成与别名同名的坏索引
// （content_vec 退化为 float、kNN 失效），且重启后 PutAlias 因同名索引已存在而永久失败——故未就绪即拒写。
func (d *ElasticArticleDAO) ensureReady() error {
	d.ensureMu.Lock()
	defer d.ensureMu.Unlock()
	if d.ensured {
		return nil
	}
	return d.ensureIndex()
}

func (d *ElasticArticleDAO) Upsert(ctx context.Context, doc ArticleESDoc) error {
	// fail-fast：别名/物理索引未就绪时拒写，避免 ES auto_create_index 建出与别名同名的坏索引（见 ensureReady）。
	if err := d.ensureReady(); err != nil {
		return fmt.Errorf("ES 索引未就绪，拒绝写入: %w", err)
	}
	_, err := d.client.Index(d.index).Id(strconv.FormatInt(doc.Id, 10)).Document(doc).Do(ctx)
	return err
}

func (d *ElasticArticleDAO) Delete(ctx context.Context, id int64) error {
	resp, err := d.client.Delete(d.index, strconv.FormatInt(id, 10)).Do(ctx)
	if err != nil {
		return err
	}
	if resp.Result.Name == "not_found" {
		return ErrESDocNotFound
	}
	return nil
}

// Search 混合检索 + 标签 facet：bool.should[title/abstract match + 向量 script_score]、
// minimum_should_match=2（保证关键词硬命中）、filter status=2；filterTags 走 post_filter（AND，只收窄 hits）；
// tags terms 聚合恒基于关键词命中集（post_filter 不影响 aggs），便于前端增删标签。
func (d *ElasticArticleDAO) Search(ctx context.Context, text string, vec []float32, filterTags []string, offset, limit int) ([]ArticleESDoc, int64, []TagCount, error) {
	query := map[string]any{
		"query": map[string]any{
			"bool": map[string]any{
				"should": []map[string]any{
					{"match": map[string]any{"title": text}},
					{"match": map[string]any{"abstract": text}},
					{"script_score": map[string]any{
						"query": map[string]any{"match_all": map[string]any{}},
						"script": map[string]any{
							"source": "cosineSimilarity(params.vec, 'content_vec') + 1.0",
							"params": map[string]any{"vec": vec},
						},
					}},
				},
				"filter":               []map[string]any{{"term": map[string]any{"status": 2}}},
				"minimum_should_match": 2,
			},
		},
		"from": offset,
		"size": limit,
		"aggs": map[string]any{
			"tags": map[string]any{"terms": map[string]any{"field": "tags", "size": 20}},
		},
	}
	if len(filterTags) > 0 {
		must := make([]map[string]any, 0, len(filterTags))
		for _, tg := range filterTags {
			must = append(must, map[string]any{"term": map[string]any{"tags": tg}})
		}
		query["post_filter"] = map[string]any{"bool": map[string]any{"must": must}}
	}
	body, err := json.Marshal(query)
	if err != nil {
		return nil, 0, nil, err
	}
	resp, err := d.client.Search().Index(d.index).Raw(bytes.NewReader(body)).TypedKeys(true).Do(ctx)
	if err != nil {
		return nil, 0, nil, err
	}
	raws := make([]json.RawMessage, 0, len(resp.Hits.Hits))
	for _, h := range resp.Hits.Hits {
		raws = append(raws, h.Source_)
	}
	docs, err := parseHits(raws)
	if err != nil {
		return nil, 0, nil, err
	}
	var total int64
	if resp.Hits.Total != nil {
		total = resp.Hits.Total.Value
	}
	facets, err := parseTagFacets(resp.Aggregations["tags"])
	if err != nil {
		return nil, 0, nil, err
	}
	return docs, total, facets, nil
}

// recommendMinScore kNN 命中最低 _score 门槛。ES cosine dense_vector 的 _score=(1+cosine)/2 ∈[0,1]，
// 0.75 ↔ cosine≈0.5（中度相关）。低于此视为语义不相关、不贡献推荐标签——
// 否则小语料/无近邻时 kNN 会把 top-k 里的远邻也塞进来（恒推热门存量标签，与新文无关）。
const recommendMinScore = 0.75

// RecommendTags kNN 取 k 篇最相似的已发布文章（filter status=2 + _score≥阈值），Go 侧聚合其标签按频次降序，
// 作 AI 推荐候选。Go 侧聚合而非 ES agg-over-knn：结果只依赖「取回哪几篇」，确定可测。
func (d *ElasticArticleDAO) RecommendTags(ctx context.Context, vec []float32, k int) ([]TagCount, error) {
	if k <= 0 {
		k = 10
	}
	numCandidates := k * 10
	if numCandidates < 50 {
		numCandidates = 50
	}
	query := map[string]any{
		"knn": map[string]any{
			"field":          "content_vec",
			"query_vector":   vec,
			"k":              k,
			"num_candidates": numCandidates,
			"filter":         map[string]any{"term": map[string]any{"status": 2}},
		},
		"size":    k,
		"_source": []string{"tags"},
	}
	body, err := json.Marshal(query)
	if err != nil {
		return nil, err
	}
	resp, err := d.client.Search().Index(d.index).Raw(bytes.NewReader(body)).Do(ctx)
	if err != nil {
		return nil, err
	}
	counts := make(map[string]int64)
	var order []string // 首见顺序，稳定排序 tiebreak
	for _, h := range resp.Hits.Hits {
		if h.Score_ == nil || float64(*h.Score_) < recommendMinScore {
			continue // 相似度不足：远邻不贡献标签（避免小语料凑不相关推荐）
		}
		var doc ArticleESDoc
		if err := json.Unmarshal(h.Source_, &doc); err != nil {
			return nil, err
		}
		for _, tg := range doc.Tags {
			if _, ok := counts[tg]; !ok {
				order = append(order, tg)
			}
			counts[tg]++
		}
	}
	out := make([]TagCount, 0, len(order))
	for _, slug := range order {
		out = append(out, TagCount{Slug: slug, Count: counts[slug]})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Count > out[j].Count })
	return out, nil
}

// parseTagFacets 解析 tags terms 聚合（TypedKeys=true 下 Aggregations["tags"] 为强类型 union）。
func parseTagFacets(agg types.Aggregate) ([]TagCount, error) {
	if agg == nil {
		return nil, nil
	}
	st, ok := agg.(*types.StringTermsAggregate)
	if !ok {
		return nil, fmt.Errorf("tags agg 类型非预期: %T", agg)
	}
	buckets, ok := st.Buckets.([]types.StringTermsBucket)
	if !ok {
		return nil, fmt.Errorf("tags buckets 类型非预期: %T", st.Buckets)
	}
	out := make([]TagCount, 0, len(buckets))
	for _, b := range buckets {
		key, ok := b.Key.(string)
		if !ok {
			continue
		}
		out = append(out, TagCount{Slug: key, Count: b.DocCount})
	}
	return out, nil
}

// parseHits 解析命中文档。
func parseHits(raw []json.RawMessage) ([]ArticleESDoc, error) {
	docs := make([]ArticleESDoc, 0, len(raw))
	for _, r := range raw {
		var doc ArticleESDoc
		if err := json.Unmarshal(r, &doc); err != nil {
			return nil, fmt.Errorf("unmarshal ES hit: %w", err)
		}
		docs = append(docs, doc)
	}
	return docs, nil
}
