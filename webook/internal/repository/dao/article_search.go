package dao

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/elastic/go-elasticsearch/v8"

	"github.com/webook/internal/errs"
)

const articleIndex = "article_v1"

// ArticleESDoc ES 文档结构
type ArticleESDoc struct {
	Id         int64     `json:"id"`
	Title      string    `json:"title"`
	Abstract   string    `json:"abstract"`
	AuthorId   int64     `json:"author_id"`
	AuthorName string    `json:"author_name"`
	Status     uint8     `json:"status"`
	CreatedAt  int64     `json:"created_at"`
	ContentVec []float32 `json:"content_vec"`
}

type ArticleSearchDAO interface {
	Upsert(ctx context.Context, doc ArticleESDoc) error
	Delete(ctx context.Context, id int64) error
	Search(ctx context.Context, text string, vec []float32, offset, limit int) ([]ArticleESDoc, int64, error)
}

type ElasticArticleDAO struct {
	client *elasticsearch.TypedClient
}

func NewElasticArticleDAO(client *elasticsearch.TypedClient) ArticleSearchDAO {
	return &ElasticArticleDAO{client: client}
}

func (d *ElasticArticleDAO) Upsert(ctx context.Context, doc ArticleESDoc) error {
	_, err := d.client.Index(articleIndex).
		Id(fmt.Sprintf("%d", doc.Id)).
		Document(doc).
		Do(ctx)
	return err
}

func (d *ElasticArticleDAO) Delete(ctx context.Context, id int64) error {
	resp, err := d.client.Delete(articleIndex, fmt.Sprintf("%d", id)).Do(ctx)
	if err != nil {
		return err
	}
	if resp.Result.Name == "not_found" {
		return errs.ErrESDocNotFound
	}
	return nil
}

// Search 混合搜索：BM25 关键词匹配（硬约束） + 向量相似度（加权排序）
// BM25 的 minimum_should_match=1 保证结果必须与关键词相关，
// script_score 用 cosineSimilarity 对向量相似的文档额外加分。
func (d *ElasticArticleDAO) Search(ctx context.Context, text string, vec []float32, offset, limit int) ([]ArticleESDoc, int64, error) {
	statusFilter := map[string]any{"term": map[string]any{"status": 2}}

	// 向量加权子句：cosineSimilarity 值域 [-1,1]，+1 偏移到 [0,2] 避免负分
	vecScore := map[string]any{
		"script_score": map[string]any{
			"query": map[string]any{"match_all": map[string]any{}},
			"script": map[string]any{
				"source": "cosineSimilarity(params.vec, 'content_vec') + 1.0",
				"params": map[string]any{"vec": vec},
			},
		},
	}

	query := map[string]any{
		"query": map[string]any{
			"bool": map[string]any{
				"should": []map[string]any{
					{"match": map[string]any{"title": text}},
					{"match": map[string]any{"abstract": text}},
					vecScore,
				},
				"filter":               statusFilter,
				"minimum_should_match": 2, // title 或 abstract 至少命中 1 个 + vecScore 恒命中 = 2
			},
		},
		"from": offset,
		"size": limit,
	}

	body, err := json.Marshal(query)
	if err != nil {
		return nil, 0, err
	}

	res, err := d.client.Search().
		Index(articleIndex).
		Raw(bytes.NewReader(body)).
		Do(ctx)
	if err != nil {
		return nil, 0, err
	}

	total := res.Hits.Total.Value
	docs := make([]ArticleESDoc, 0, len(res.Hits.Hits))
	for _, hit := range res.Hits.Hits {
		var doc ArticleESDoc
		if err = json.Unmarshal(hit.Source_, &doc); err != nil {
			return nil, 0, fmt.Errorf("unmarshal ES hit: %w", err)
		}
		docs = append(docs, doc)
	}
	return docs, total, nil
}
