package ioc

import (
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"

	"github.com/webook/pkg/llm"
	"github.com/webook/pkg/logger"
	"github.com/webook/pkg/ratelimit"
)

func InitLLMConfig() llm.Config {
	var cfg llm.Config
	if err := viper.UnmarshalKey("llm", &cfg); err != nil {
		panic("读取 LLM 配置失败: " + err.Error())
	}
	if len(cfg.Providers) == 0 {
		panic("至少需要配置一个 LLM provider")
	}
	return cfg
}

func InitLLMClient(cfg llm.Config, l logger.LoggerX) llm.Client {
	clients := make([]llm.Client, 0, len(cfg.Providers))
	for _, p := range cfg.Providers {
		clients = append(clients, llm.NewOpenAIClient(p))
	}
	if len(clients) == 1 {
		return clients[0]
	}
	return llm.NewTimeoutFailoverClient(clients, 3, l)
}

// InitChatLimiter 聊天发送限流：10 条/分钟
func InitChatLimiter(cmd redis.Cmdable) ratelimit.Limiter {
	return ratelimit.NewRedisSlidingWindowLimiter(cmd, time.Minute, 10)
}
