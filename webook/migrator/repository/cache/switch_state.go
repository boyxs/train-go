package cache

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// 切流状态 Redis 键。key 按 task.Name 而非 taskId，跟 SDK [internal/migratorsdk/redis.go]
// 对齐 —— 业务方只从 yaml migrator.sdk.taskName 拿到 name，不知道 ID。
const (
	// KeyStage Stage key 前缀 → string Stage。
	KeyStage = "migrator:stage:"
	// KeyGray 灰度比例 key 前缀 → int percent。
	KeyGray = "migrator:gray:"
	// KeyCutoverPropose 双人复核 propose 临时 key 前缀 → propose actor_id。
	KeyCutoverPropose = "migrator:cutover_propose:"
	// CutoverProposeTTL propose 有效期，过期需重新 propose。
	CutoverProposeTTL = 10 * time.Minute
)

// SwitchStateCache 切流状态存储。Redis 是路由决策真相源（SDK 直接读这些键），
// 不是 DB 的缓存。Get* 键不存在时返回零值 + nil error（stage=""、gray=0、propose=""）。
type SwitchStateCache interface {
	SetGray(ctx context.Context, taskName string, percent int) error
	GetGray(ctx context.Context, taskName string) (int, error)
	SetStage(ctx context.Context, taskName, stage string) error
	GetStage(ctx context.Context, taskName string) (string, error)
	// SetPropose 注册 propose actor，TTL = CutoverProposeTTL。
	SetPropose(ctx context.Context, taskName, actor string) error
	GetPropose(ctx context.Context, taskName string) (string, error)
	DelPropose(ctx context.Context, taskName string) error
}

type RedisSwitchStateCache struct {
	cmd redis.Cmdable
}

func NewRedisSwitchStateCache(cmd redis.Cmdable) SwitchStateCache {
	return &RedisSwitchStateCache{cmd: cmd}
}

func (c *RedisSwitchStateCache) SetGray(ctx context.Context, taskName string, percent int) error {
	return c.cmd.Set(ctx, KeyGray+taskName, percent, 0).Err()
}

func (c *RedisSwitchStateCache) GetGray(ctx context.Context, taskName string) (int, error) {
	v, err := c.cmd.Get(ctx, KeyGray+taskName).Int()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	return v, err
}

func (c *RedisSwitchStateCache) SetStage(ctx context.Context, taskName, stage string) error {
	return c.cmd.Set(ctx, KeyStage+taskName, stage, 0).Err()
}

func (c *RedisSwitchStateCache) GetStage(ctx context.Context, taskName string) (string, error) {
	v, err := c.cmd.Get(ctx, KeyStage+taskName).Result()
	if errors.Is(err, redis.Nil) {
		return "", nil
	}
	return v, err
}

func (c *RedisSwitchStateCache) SetPropose(ctx context.Context, taskName, actor string) error {
	return c.cmd.Set(ctx, KeyCutoverPropose+taskName, actor, CutoverProposeTTL).Err()
}

func (c *RedisSwitchStateCache) GetPropose(ctx context.Context, taskName string) (string, error) {
	v, err := c.cmd.Get(ctx, KeyCutoverPropose+taskName).Result()
	if errors.Is(err, redis.Nil) {
		return "", nil
	}
	return v, err
}

func (c *RedisSwitchStateCache) DelPropose(ctx context.Context, taskName string) error {
	return c.cmd.Del(ctx, KeyCutoverPropose+taskName).Err()
}
