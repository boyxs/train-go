package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/boyxs/train-go/webook/chat/consts"
	"github.com/boyxs/train-go/webook/chat/domain"
)

const (
	chatConvTTL    = 10 * time.Minute
	chatConvJitter = 2 * time.Minute
)

type ConversationCache interface {
	GetList(ctx context.Context, uid int64) ([]domain.Conversation, error)
	SetList(ctx context.Context, uid int64, convs []domain.Conversation) error
	Del(ctx context.Context, uid int64) error
	// Get 单条对话缓存（key 含 uid，越权访问 → key 不存在 → miss 回源 DAO 二次校验）
	Get(ctx context.Context, uid int64, convId int64) (domain.Conversation, error)
	Set(ctx context.Context, conv domain.Conversation) error
	DelOne(ctx context.Context, uid int64, convId int64) error
}

type RedisConversationCache struct {
	cmd redis.Cmdable
}

func NewRedisConversationCache(cmd redis.Cmdable) ConversationCache {
	return &RedisConversationCache{cmd: cmd}
}

func (c *RedisConversationCache) GetList(ctx context.Context, uid int64) ([]domain.Conversation, error) {
	data, err := c.cmd.Get(ctx, c.listKey(uid)).Bytes()
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
	return c.cmd.Set(ctx, c.listKey(uid), data, ttl).Err()
}

func (c *RedisConversationCache) Del(ctx context.Context, uid int64) error {
	return c.cmd.Del(ctx, c.listKey(uid)).Err()
}

func (c *RedisConversationCache) Get(ctx context.Context, uid int64, convId int64) (domain.Conversation, error) {
	data, err := c.cmd.Get(ctx, c.itemKey(uid, convId)).Bytes()
	if err != nil {
		return domain.Conversation{}, err
	}
	var conv domain.Conversation
	if err = json.Unmarshal(data, &conv); err != nil {
		return domain.Conversation{}, err
	}
	return conv, nil
}

func (c *RedisConversationCache) Set(ctx context.Context, conv domain.Conversation) error {
	data, err := json.Marshal(conv)
	if err != nil {
		return err
	}
	ttl := chatConvTTL + time.Duration(rand.Int63n(int64(chatConvJitter)))
	return c.cmd.Set(ctx, c.itemKey(conv.UserId, conv.Id), data, ttl).Err()
}

func (c *RedisConversationCache) DelOne(ctx context.Context, uid int64, convId int64) error {
	return c.cmd.Del(ctx, c.itemKey(uid, convId)).Err()
}

func (c *RedisConversationCache) listKey(uid int64) string {
	return fmt.Sprintf(consts.ChatConvPattern, uid)
}

func (c *RedisConversationCache) itemKey(uid int64, convId int64) string {
	return fmt.Sprintf(consts.ChatConvItemPattern, uid, convId)
}
