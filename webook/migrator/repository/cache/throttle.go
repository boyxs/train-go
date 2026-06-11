package cache

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/redis/go-redis/v9"
)

// ThrottleConfig 一组限速配置。
type ThrottleConfig struct {
	QPS          int `json:"qps"`
	ShardWorkers int `json:"shard_workers"`
}

// Empty true 表示该 task 没有自定义 throttle（用引擎默认值）。
func (t ThrottleConfig) Empty() bool {
	return t.QPS <= 0 && t.ShardWorkers <= 0
}

// ThrottleCache 持久化 task 级限速配置。
//
// 设计目标：handler 通过接口注入，**不感知具体存储**（Redis / 未来可换 etcd / MySQL）。
type ThrottleCache interface {
	// Get 未配置返回 (zero, false, nil)；存在返回 (cfg, true, nil)。
	Get(ctx context.Context, taskId int64) (ThrottleConfig, bool, error)
	// Set 写入配置（无 TTL — 配置一直生效到下次手动 Clear 或覆写）。
	Set(ctx context.Context, taskId int64, cfg ThrottleConfig) error
	// Clear 清空配置 — 下次启动恢复引擎默认值。
	Clear(ctx context.Context, taskId int64) error
}

// RedisThrottleKeyPrefix Redis key 前缀，方便 ops 端按业务过滤。
const RedisThrottleKeyPrefix = "migrator:throttle:"

// RedisThrottleCache ThrottleCache 的 Redis 实现。
type RedisThrottleCache struct {
	cmd redis.Cmdable
}

func NewRedisThrottleCache(cmd redis.Cmdable) ThrottleCache {
	return &RedisThrottleCache{cmd: cmd}
}

func (c *RedisThrottleCache) Get(ctx context.Context, taskId int64) (ThrottleConfig, bool, error) {
	raw, err := c.cmd.Get(ctx, c.key(taskId)).Bytes()
	if errors.Is(err, redis.Nil) {
		return ThrottleConfig{}, false, nil
	}
	if err != nil {
		return ThrottleConfig{}, false, err
	}
	var cfg ThrottleConfig
	if uerr := json.Unmarshal(raw, &cfg); uerr != nil {
		return ThrottleConfig{}, false, uerr
	}
	return cfg, true, nil
}

func (c *RedisThrottleCache) Set(ctx context.Context, taskId int64, cfg ThrottleConfig) error {
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	// TTL=0 永久（配置一直生效）；Clear 显式删除
	return c.cmd.Set(ctx, c.key(taskId), raw, 0).Err()
}

func (c *RedisThrottleCache) Clear(ctx context.Context, taskId int64) error {
	return c.cmd.Del(ctx, c.key(taskId)).Err()
}

func (c *RedisThrottleCache) key(taskId int64) string {
	return RedisThrottleKeyPrefix + strconv.FormatInt(taskId, 10)
}
