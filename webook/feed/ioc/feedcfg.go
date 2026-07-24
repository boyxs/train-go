package ioc

import (
	"time"

	"github.com/spf13/viper"

	"github.com/boyxs/train-go/webook/feed/repository/cache"
	"github.com/boyxs/train-go/webook/feed/service"
)

// feedTuning 映射 yaml `feed` 业务段（逐环境可调）。
type feedTuning struct {
	BigVThreshold       int64         `mapstructure:"big_v_threshold"`
	InboxCap            int64         `mapstructure:"inbox_cap"`
	InboxTTL            time.Duration `mapstructure:"inbox_ttl"`
	OutboxSize          int64         `mapstructure:"outbox_size"`
	OutboxTTL           time.Duration `mapstructure:"outbox_ttl"`
	RebuildMaxFollowees int64         `mapstructure:"rebuild_max_followees"`
	FanoutBatch         int           `mapstructure:"fanout_batch"`
	NewCountMax         int           `mapstructure:"new_count_max"`
}

func loadFeedTuning() feedTuning {
	// 缺省兜底（对齐 config yaml），yaml 覆盖。
	t := feedTuning{
		BigVThreshold: 1000, InboxCap: 2000, InboxTTL: 168 * time.Hour,
		OutboxSize: 100, OutboxTTL: time.Hour, RebuildMaxFollowees: 1000, FanoutBatch: 50,
		NewCountMax: 99,
	}
	if err := viper.UnmarshalKey("feed", &t); err != nil {
		panic(err)
	}
	return t
}

// InitCacheConfig 从 `feed` 段构建 cache 调参。
func InitCacheConfig() cache.Config {
	t := loadFeedTuning()
	return cache.Config{
		InboxCap:   t.InboxCap,
		InboxTTL:   t.InboxTTL,
		OutboxSize: t.OutboxSize,
		OutboxTTL:  t.OutboxTTL,
	}
}

// InitServiceConfig 从 `feed` 段构建 service 调参。
func InitServiceConfig() service.Config {
	t := loadFeedTuning()
	return service.Config{
		BigVThreshold:       t.BigVThreshold,
		FanoutBatch:         t.FanoutBatch,
		RebuildMaxFollowees: t.RebuildMaxFollowees,
		OutboxSize:          int(t.OutboxSize),
		NewCountMax:         t.NewCountMax,
	}
}
