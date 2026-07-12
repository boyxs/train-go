package ioc

import (
	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"

	"github.com/boyxs/train-go/webook/pkg/embedding"
	"github.com/boyxs/train-go/webook/pkg/logger"
)

func InitEmbeddingConfig() embedding.Config {
	var cfg embedding.Config
	if err := viper.UnmarshalKey("embedding", &cfg); err != nil {
		panic("读取 embedding 配置失败: " + err.Error())
	}
	if cfg.BaseUrl == "" {
		panic("embedding.base_url 不能为空")
	}
	return cfg
}

func InitOllamaEmbeddingConfig() embedding.OllamaConfig {
	var cfg embedding.OllamaConfig
	if err := viper.UnmarshalKey("ollama", &cfg); err != nil {
		// Ollama 可选，读取失败返回零值，不 panic
		return embedding.OllamaConfig{}
	}
	return cfg
}

// InitEmbeddingClient 组装向量化客户端：本地 Ollama 优先 → 收费 API 兜底，外层加 Redis 缓存。
func InitEmbeddingClient(
	ollamaCfg embedding.OllamaConfig,
	embCfg embedding.Config,
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

	return embedding.NewCachedClient(raw, cmd, l)
}
