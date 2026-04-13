package saramax

import "github.com/IBM/sarama"

// Handler 单条消息处理函数
type Handler[T any] func(msg *sarama.ConsumerMessage, event T) error

// BatchHandler 批量消息处理函数（msgs 和 events 一一对应，长度相同）
type BatchHandler[T any] func(msgs []*sarama.ConsumerMessage, events []T) error
