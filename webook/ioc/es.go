package ioc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/webook/config"
	"github.com/webook/internal/repository/cache"
	"github.com/webook/internal/service/ai/embedding"
	"github.com/webook/pkg/logger"
	"github.com/elastic/go-elasticsearch/v8"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"
)

func InitESClient() *elasticsearch.TypedClient {
	var addr string
	if err := viper.UnmarshalKey("es.addr", &addr); err != nil || addr == "" {
		addr = "http://localhost:9200"
	}
	client, err := elasticsearch.NewTypedClient(elasticsearch.Config{
		Addresses: []string{addr},
	})
	if err != nil {
		panic("初始化 ES 客户端失败: " + err.Error())
	}
	ensureArticleIndex(client)
	return client
}

// ensureArticleIndex 若索引不存在则创建，含 dense_vector mapping（kNN 搜索必须）。
// 若索引已存在（可能是旧 mapping）则跳过；需手动 DELETE /article_v1 后重启以重建。
func ensureArticleIndex(client *elasticsearch.TypedClient) {
	const indexName = "article_v1"
	ctx := context.Background()

	exists, err := client.Indices.Exists(indexName).Do(ctx)
	if err != nil {
		fmt.Printf("[ES] 检查索引失败，跳过建索引: %v\n", err)
		return
	}
	if exists {
		return
	}

	body, _ := json.Marshal(map[string]any{
		"mappings": map[string]any{
			"properties": map[string]any{
				"id":          map[string]any{"type": "long"},
				"title":       map[string]any{"type": "text"},
				"abstract":    map[string]any{"type": "text"},
				"author_id":   map[string]any{"type": "long"},
				"author_name": map[string]any{"type": "keyword"},
				"status":      map[string]any{"type": "byte"},
				"created_at":  map[string]any{"type": "date", "format": "epoch_millis"},
				"content_vec": map[string]any{
					"type":       "dense_vector",
					"dims":       1024,
					"index":      true,
					"similarity": "cosine",
				},
			},
		},
	})

	if _, err = client.Indices.Create(indexName).Raw(bytes.NewReader(body)).Do(ctx); err != nil {
		fmt.Printf("[ES] 创建索引失败: %v\n", err)
	}
}

func InitEmbeddingConfig() config.EmbeddingConfig {
	var cfg config.EmbeddingConfig
	if err := viper.UnmarshalKey("embedding", &cfg); err != nil {
		panic("读取 embedding 配置失败: " + err.Error())
	}
	if cfg.BaseUrl == "" {
		panic("embedding.base_url 不能为空")
	}
	return cfg
}

func InitOllamaEmbeddingConfig() config.OllamaEmbeddingConfig {
	var cfg config.OllamaEmbeddingConfig
	if err := viper.UnmarshalKey("ollama", &cfg); err != nil {
		// Ollama 可选，读取失败返回零值，不 panic
		return config.OllamaEmbeddingConfig{}
	}
	return cfg
}

func InitEmbeddingClient(
	ollamaCfg config.OllamaEmbeddingConfig,
	embCfg config.EmbeddingConfig,
	cmd redis.Cmdable,
	l logger.LoggerX,
) embedding.EmbeddingClient {
	// 收费 API 作为兜底（必须有）
	openaiClient := embedding.NewOpenAIClient(embCfg)

	var raw embedding.EmbeddingClient
	if ollamaCfg.BaseUrl != "" {
		// 本地 Ollama 优先，降级到收费
		ollamaClient := embedding.NewOllamaClient(ollamaCfg)
		raw = embedding.NewFailoverClient(
			[]embedding.EmbeddingClient{ollamaClient, openaiClient}, l,
		)
	} else {
		raw = openaiClient
	}

	return cache.NewCachedEmbeddingClient(raw, cmd, l)
}
