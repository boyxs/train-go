package streamer

import (
	"context"
	"time"
)

// EventStreamer 事件流抽象，用于 SSE 断线续传
type EventStreamer interface {
	// Publish 写入一条事件，返回事件 ID
	Publish(ctx context.Context, key string, data string) (string, error)
	// ReadAfter 读取 afterId 之后的事件（非阻塞），返回 (data[], id[])
	ReadAfter(ctx context.Context, key string, afterId string) ([]string, []string, error)
	// BlockRead 阻塞读取 afterId 之后的新事件，timeout 到期返回空
	BlockRead(ctx context.Context, key string, afterId string, timeout time.Duration) ([]string, []string, error)
	// Expire 设置 key 过期时间
	Expire(ctx context.Context, key string, ttl time.Duration) error
}
