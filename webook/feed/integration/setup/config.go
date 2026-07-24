package setup

import (
	"time"

	"github.com/boyxs/train-go/webook/feed/repository/cache"
	"github.com/boyxs/train-go/webook/feed/service"
)

// InitCacheConfig 集成测试固定缓存调参（与 config/test.yaml 的 feed 段一致）。
func InitCacheConfig() cache.Config {
	return cache.Config{InboxCap: 2000, InboxTTL: 168 * time.Hour, OutboxSize: 100, OutboxTTL: time.Hour}
}

// InitServiceConfig 集成测试固定业务调参。
func InitServiceConfig() service.Config {
	return service.Config{BigVThreshold: 1000, FanoutBatch: 50, RebuildMaxFollowees: 1000, OutboxSize: 100}
}
