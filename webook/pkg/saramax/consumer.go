package saramax

import (
	"encoding/json"

	"gitee.com/train-cloud/geektime-basic-go/pkg/logger"
	"github.com/IBM/sarama"
)

// Consumer 单条消费者，实现 sarama.ConsumerGroupHandler 接口
// 使用方只需关心业务 Handler，无需处理 JSON 反序列化和 offset 提交
type Consumer[T any] struct {
	handler Handler[T]
	l       logger.LoggerX
}

func NewConsumer[T any](handler Handler[T], l logger.LoggerX) *Consumer[T] {
	return &Consumer[T]{handler: handler, l: l}
}

func (c *Consumer[T]) Setup(_ sarama.ConsumerGroupSession) error   { return nil }
func (c *Consumer[T]) Cleanup(_ sarama.ConsumerGroupSession) error { return nil }

func (c *Consumer[T]) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for msg := range claim.Messages() {
		var event T
		if err := json.Unmarshal(msg.Value, &event); err != nil {
			c.l.Error("反序列化消息失败",
				logger.String("topic", msg.Topic),
				logger.Int64("partition", int64(msg.Partition)),
				logger.Int64("offset", msg.Offset),
				logger.Error(err))
			// 反序列化失败：标记消费避免阻塞，业务上下游需要监控这种情况
			session.MarkMessage(msg, "")
			continue
		}
		if err := c.handler(msg, event); err != nil {
			c.l.Error("处理消息失败",
				logger.String("topic", msg.Topic),
				logger.Int64("offset", msg.Offset),
				logger.Error(err))
			// 处理失败：不标记，下次重新消费
			continue
		}
		session.MarkMessage(msg, "")
	}
	return nil
}
