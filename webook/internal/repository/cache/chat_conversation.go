package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"gitee.com/train-cloud/geektime-basic-go/internal/consts"
	"gitee.com/train-cloud/geektime-basic-go/internal/domain"
	"github.com/redis/go-redis/v9"
)

const (
	chatConvTTL    = 10 * time.Minute
	chatConvJitter = 2 * time.Minute
)

type ConversationCache interface {
	GetList(ctx context.Context, uid int64) ([]domain.Conversation, error)
	SetList(ctx context.Context, uid int64, convs []domain.Conversation) error
	Del(ctx context.Context, uid int64) error
}

type RedisConversationCache struct {
	cmd redis.Cmdable
}

func NewRedisConversationCache(cmd redis.Cmdable) ConversationCache {
	return &RedisConversationCache{cmd: cmd}
}

func (c *RedisConversationCache) GetList(ctx context.Context, uid int64) ([]domain.Conversation, error) {
	data, err := c.cmd.Get(ctx, c.key(uid)).Bytes()
	if err != nil {
		return nil, err
	}
	var convs []domain.Conversation
	if err = json.Unmarshal(data, &convs); err != nil {
		return nil, err
	}
	return convs, nil
}

func (c *RedisConversationCache) SetList(ctx context.Context, uid int64, convs []domain.Conversation) error {
	data, err := json.Marshal(convs)
	if err != nil {
		return err
	}
	ttl := chatConvTTL + time.Duration(rand.Int63n(int64(chatConvJitter)))
	return c.cmd.Set(ctx, c.key(uid), data, ttl).Err()
}

func (c *RedisConversationCache) Del(ctx context.Context, uid int64) error {
	return c.cmd.Del(ctx, c.key(uid)).Err()
}

func (c *RedisConversationCache) key(uid int64) string {
	return fmt.Sprintf(consts.ChatConvPattern, uid)
}
