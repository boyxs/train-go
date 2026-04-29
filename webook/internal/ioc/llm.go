package ioc

import (
	"github.com/spf13/viper"

	"github.com/webook/pkg/llm"
	"github.com/webook/pkg/logger"
)

// LLM provider 配置 + 客户端构造。供 article_polish 等需要 LLM 的模块使用。
// 注：原 InitChatLimiter 已随 chat 服务搬到 chat/ioc/llm.go。

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
