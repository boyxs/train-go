package streamer

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisStreamer 基于 Redis Stream 的 EventStreamer 实现
type RedisStreamer struct {
	cmd    redis.Cmdable
	maxLen int64
}

func NewRedisStreamer(cmd redis.Cmdable) EventStreamer {
	return &RedisStreamer{cmd: cmd, maxLen: 1000}
}

func (s *RedisStreamer) Publish(ctx context.Context, key string, data string) (string, error) {
	return s.cmd.XAdd(ctx, &redis.XAddArgs{
		Stream: key,
		MaxLen: s.maxLen,
		Approx: true,
		Values: map[string]any{"event": data},
	}).Result()
}

func (s *RedisStreamer) ReadAfter(ctx context.Context, key string, afterId string) ([]string, []string, error) {
	if afterId == "" {
		afterId = "0"
	}
	msgs, err := s.cmd.XRange(ctx, key, afterId, "+").Result()
	if err != nil {
		return nil, nil, err
	}
	// afterId 是上次读到的 ID，XRange 包含它，跳过
	start := 0
	if afterId != "0" && len(msgs) > 0 && msgs[0].ID == afterId {
		start = 1
	}
	events := make([]string, 0, len(msgs)-start)
	ids := make([]string, 0, len(msgs)-start)
	for _, m := range msgs[start:] {
		raw, _ := m.Values["event"].(string)
		events = append(events, raw)
		ids = append(ids, m.ID)
	}
	return events, ids, nil
}

func (s *RedisStreamer) BlockRead(ctx context.Context, key string, afterId string, timeout time.Duration) ([]string, []string, error) {
	if afterId == "" {
		afterId = "$" // XREAD 的 $ 表示只读新消息
	}
	result, err := s.cmd.XRead(ctx, &redis.XReadArgs{
		Streams: []string{key, afterId},
		Block:   timeout,
		Count:   100,
	}).Result()
	if err != nil {
		return nil, nil, err // 超时也返回 err (redis.Nil)
	}
	if len(result) == 0 || len(result[0].Messages) == 0 {
		return nil, nil, nil
	}
	msgs := result[0].Messages
	events := make([]string, 0, len(msgs))
	ids := make([]string, 0, len(msgs))
	for _, m := range msgs {
		raw, _ := m.Values["event"].(string)
		events = append(events, raw)
		ids = append(ids, m.ID)
	}
	return events, ids, nil
}

func (s *RedisStreamer) Expire(ctx context.Context, key string, ttl time.Duration) error {
	return s.cmd.Expire(ctx, key, ttl).Err()
}
