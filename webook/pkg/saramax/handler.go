package saramax

import (
	"context"

	"github.com/IBM/sarama"
)

// Handler 单条消息处理函数
// ctx 携带 OTel trace context，下游 service 层 span 可自动挂到 consumer span 下
type Handler[T any] func(ctx context.Context, msg *sarama.ConsumerMessage, event T) error

// BatchHandler 批量消息处理函数（msgs 和 events 一一对应，长度相同）
// ctx 从批次第一条消息的 Kafka headers 提取 trace context 而来，用它调用下游保持链路连续
type BatchHandler[T any] func(ctx context.Context, msgs []*sarama.ConsumerMessage, events []T) error
