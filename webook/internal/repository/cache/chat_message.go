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
	chatMsgTTL    = 10 * time.Minute
	chatMsgJitter = 2 * time.Minute
)

type MessageCache interface {
	GetList(ctx context.Context, convId int64) ([]domain.Message, error)
	SetList(ctx context.Context, convId int64, msgs []domain.Message) error
	Del(ctx context.Context, convId int64) error
}

type RedisMessageCache struct {
	cmd redis.Cmdable
}

func NewRedisMessageCache(cmd redis.Cmdable) MessageCache {
	return &RedisMessageCache{cmd: cmd}
}

func (c *RedisMessageCache) GetList(ctx context.Context, convId int64) ([]domain.Message, error) {
	data, err := c.cmd.Get(ctx, c.key(convId)).Bytes()
	if err != nil {
		return nil, err
	}
	var msgs []domain.Message
	if err = json.Unmarshal(data, &msgs); err != nil {
		return nil, err
	}
	return msgs, nil
}

func (c *RedisMessageCache) SetList(ctx context.Context, convId int64, msgs []domain.Message) error {
	data, err := json.Marshal(msgs)
	if err != nil {
		return err
	}
	ttl := chatMsgTTL + time.Duration(rand.Int63n(int64(chatMsgJitter)))
	return c.cmd.Set(ctx, c.key(convId), data, ttl).Err()
}

func (c *RedisMessageCache) Del(ctx context.Context, convId int64) error {
	return c.cmd.Del(ctx, c.key(convId)).Err()
}

func (c *RedisMessageCache) key(convId int64) string {
	return fmt.Sprintf(consts.ChatMsgPattern, convId)
}
