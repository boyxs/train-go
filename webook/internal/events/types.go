package events

import "context"

// Producer 通用消息生产者接口，所有业务共用
// sync/async 都实现此接口，调用方无感知
type Producer interface {
	ProduceEvent(ctx context.Context, topic string, key string, value []byte) error
}

// Consumer 通用消费者接口，每个业务实现自己的 Handler
type Consumer interface {
	Start(ctx context.Context) error
}
