package saramax

import (
	"context"
	"encoding/json"
	"time"

	"github.com/IBM/sarama"

	"github.com/webook/pkg/logger"
)

// BatchConsumer 批量消费者
// 攒够 batchSize 条或超时 batchTimeout 后调用一次 handler
// handler 成功后批量提交 offset；失败则整批不提交，下次重新消费
type BatchConsumer[T any] struct {
	handler      BatchHandler[T]
	batchSize    int
	batchTimeout time.Duration
	l            logger.LoggerX
}

// NewBatchConsumer batchSize<=0 默认 10，batchTimeout<=0 默认 1s
func NewBatchConsumer[T any](
	handler BatchHandler[T],
	batchSize int,
	batchTimeout time.Duration,
	l logger.LoggerX,
) *BatchConsumer[T] {
	if batchSize <= 0 {
		batchSize = 10
	}
	if batchTimeout <= 0 {
		batchTimeout = time.Second
	}
	return &BatchConsumer[T]{
		handler:      handler,
		batchSize:    batchSize,
		batchTimeout: batchTimeout,
		l:            l,
	}
}

func (c *BatchConsumer[T]) Setup(_ sarama.ConsumerGroupSession) error   { return nil }
func (c *BatchConsumer[T]) Cleanup(_ sarama.ConsumerGroupSession) error { return nil }

func (c *BatchConsumer[T]) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	msgCh := claim.Messages()
	for {
		msgs := make([]*sarama.ConsumerMessage, 0, c.batchSize)
		events := make([]T, 0, c.batchSize)
		ctx, cancel := context.WithTimeout(context.Background(), c.batchTimeout)

		// 攒批：达到 batchSize 或超时
		done := false
		for i := 0; i < c.batchSize; i++ {
			select {
			case <-ctx.Done():
				i = c.batchSize // 跳出循环
			case msg, ok := <-msgCh:
				if !ok {
					done = true
					i = c.batchSize
					break
				}
				var event T
				if err := json.Unmarshal(msg.Value, &event); err != nil {
					c.l.Error("反序列化消息失败，标记消费避免阻塞",
						logger.String("topic", msg.Topic),
						logger.Int64("offset", msg.Offset),
						logger.Error(err))
					// 反序列化失败：标记消费避免下次重启反复读到坏消息
					session.MarkMessage(msg, "")
					continue
				}
				msgs = append(msgs, msg)
				events = append(events, event)
			}
		}
		cancel()

		// 处理批次：成功提交 offset，失败整批重试
		if len(msgs) > 0 {
			if err := c.handler(msgs, events); err != nil {
				c.l.Error("批量处理消息失败",
					logger.Int64("size", int64(len(msgs))), logger.Error(err))
			} else {
				for _, msg := range msgs {
					session.MarkMessage(msg, "")
				}
			}
		}

		if done {
			return nil
		}
	}
}
