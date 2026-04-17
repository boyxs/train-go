package ioc

import (
	"time"

	"github.com/webook/config"
	"github.com/webook/internal/service/ai"
	"github.com/webook/pkg/logger"
	"github.com/webook/pkg/ratelimit"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"
)

func InitLLMConfig() config.LLMConfig {
	var cfg config.LLMConfig
	if err := viper.UnmarshalKey("llm", &cfg); err != nil {
		panic("读取 LLM 配置失败: " + err.Error())
	}
	if len(cfg.Providers) == 0 {
		panic("至少需要配置一个 LLM provider")
	}
	return cfg
}

func InitLLMClient(cfg config.LLMConfig, l logger.LoggerX) ai.LLMClient {
	clients := make([]ai.LLMClient, 0, len(cfg.Providers))
	for _, p := range cfg.Providers {
		clients = append(clients, ai.NewOpenAIClient(p))
	}
	if len(clients) == 1 {
		return clients[0]
	}
	// FailoverClient 严格轮询
	//return ai.NewFailoverClient(clients, l)
	// TimeoutFailover 包裹多个 provider：
	// 粘性使用主 provider，连续 3 次故障自动切换
	return ai.NewTimeoutFailoverClient(clients, 3, l)
}

// InitChatLimiter 聊天发送限流：10 条/分钟，复用 pkg/ratelimit
func InitChatLimiter(cmd redis.Cmdable) ratelimit.Limiter {
	return ratelimit.NewRedisSlidingWindowLimiter(cmd, time.Minute, 10)
}
