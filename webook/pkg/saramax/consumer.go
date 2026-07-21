package saramax

import (
	"context"
	"encoding/json"

	"github.com/IBM/sarama"

	"github.com/boyxs/train-go/webook/pkg/logger"
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
			c.l.WithContext(ExtractTraceContext(msg)).Error("反序列化消息失败",
				logger.String("topic", msg.Topic),
				logger.Int64("partition", int64(msg.Partition)),
				logger.Int64("offset", msg.Offset),
				logger.Error(err))
			// 反序列化失败：标记消费避免阻塞，业务上下游需要监控这种情况
			session.MarkMessage(msg, "")
			continue
		}
		c.handleOne(session, msg, event)
	}
	return nil
}

// handleOne 单条处理：extract → start span → defer End → handle
// 抽出来是为了 defer span.End() 覆盖 handler panic 场景
func (c *Consumer[T]) handleOne(session sarama.ConsumerGroupSession, msg *sarama.ConsumerMessage, event T) {
	ctx, span := StartConsumerSpan(context.Background(), msg)
	defer span.End()
	if err := c.handler(ctx, msg, event); err != nil {
		RecordSpanError(span, err)
		c.l.WithContext(ctx).Error("处理消息失败",
			logger.String("topic", msg.Topic),
			logger.Int64("offset", msg.Offset),
			logger.Error(err))
		// 处理失败：不标记，下次重新消费
		return
	}
	session.MarkMessage(msg, "")
}
