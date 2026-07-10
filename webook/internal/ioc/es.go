package ioc

import (
	"github.com/elastic/go-elasticsearch/v9"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"

	"github.com/boyxs/train-go/webook/internal/repository/cache"
	"github.com/boyxs/train-go/webook/internal/service/embedding"
	"github.com/boyxs/train-go/webook/pkg/logger"
)

// esConfig core 的 ES 连接配置（对齐 migrator.es 与 pkg/redisx 风格）：
// addrs 支持多节点即集群；username/password 走 ${ES_PASS}，连启用 xpack.security 的 ES。
// 目标 ES 未开安全时带上凭据也无害（服务端忽略），故 5 份 yaml 一律带 auth。
type esConfig struct {
	Addrs    []string `mapstructure:"addrs"`
	Username string   `mapstructure:"username"`
	Password string   `mapstructure:"password"`
}

func InitESClient(l logger.LoggerX) *elasticsearch.TypedClient {
	var cfg esConfig
	if err := viper.UnmarshalKey("data.es", &cfg); err != nil {
		panic("读取 data.es 配置失败: " + err.Error())
	}
	if len(cfg.Addrs) == 0 {
		l.Error("data.es.addrs 未配置，ES 客户端无法初始化")
		panic("data.es.addrs 未配置")
	}
	// go-elasticsearch v9 起用函数式 Option（NewTypedClient/Config 已弃用）。
	opts := []elasticsearch.Option{
		elasticsearch.WithAddresses(cfg.Addrs...),
		elasticsearch.WithBasicAuth(cfg.Username, cfg.Password),
	}
	client, err := elasticsearch.NewTyped(opts...)
	if err != nil {
		panic("初始化 ES 客户端失败: " + err.Error())
	}
	return client
}

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

func InitEmbeddingClient(
	ollamaCfg embedding.OllamaConfig,
	embCfg embedding.Config,
	cmd redis.Cmdable,
	l logger.LoggerX,
) embedding.Client {
	// 收费 API 作为兜底（必须有）
	openaiClient := embedding.NewOpenAIClient(embCfg)

	var raw embedding.Client
	if ollamaCfg.BaseUrl != "" {
		// 本地 Ollama 优先，降级到收费
		ollamaClient := embedding.NewOllamaClient(ollamaCfg)
		raw = embedding.NewFailoverClient(
			[]embedding.Client{ollamaClient, openaiClient}, l,
		)
	} else {
		raw = openaiClient
	}

	return cache.NewCachedEmbeddingClient(raw, cmd, l)
}
