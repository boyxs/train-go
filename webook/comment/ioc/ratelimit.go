package ioc

import (
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"

	"github.com/webook/pkg/ratelimit"
)

// InitLimiter 评论发表限流（Redis 滑动窗口）。窗口/阈值从 yaml ratelimit.comment，缺省 1 分钟 30 次。
func InitLimiter(cmd redis.Cmdable) ratelimit.Limiter {
	type Config struct {
		Interval time.Duration `yaml:"interval"`
		Rate     int           `yaml:"rate"`
	}
	cfg := Config{
		Interval: time.Minute,
		Rate:     30,
	}
	if err := viper.UnmarshalKey("ratelimit.comment", &cfg); err != nil {
		panic(err)
	}
	return ratelimit.NewRedisSlidingWindowLimiter(cmd, cfg.Interval, cfg.Rate)
}
