package events

import (
	"context"
	"fmt"

	"github.com/IBM/sarama"

	"github.com/webook/pkg/logger"
	"github.com/webook/pkg/saramax"
)

// SaramaSyncProducer 同步生产者，发送后等待 broker 确认
type SaramaSyncProducer struct {
	producer sarama.SyncProducer
	l        logger.LoggerX
}

func NewSaramaSyncProducer(producer sarama.SyncProducer, l logger.LoggerX) Producer {
	return &SaramaSyncProducer{producer: producer, l: l}
}

func (p *SaramaSyncProducer) ProduceEvent(ctx context.Context, topic string, key string, value []byte) error {
	msg := &sarama.ProducerMessage{
		Topic: topic,
		Key:   sarama.StringEncoder(key),
		Value: sarama.ByteEncoder(value),
	}
	// OTel：创建 Producer span + 把 trace context 注入 Kafka headers，
	// consumer 端 Extract 后可挂上同一 trace
	_, span := saramax.StartProducerSpan(ctx, msg)
	defer span.End()

	partition, offset, err := p.producer.SendMessage(msg)
	if err != nil {
		saramax.RecordSpanError(span, err)
		return fmt.Errorf("kafka sync send failed: topic=%s key=%s err=%w", topic, key, err)
	}
	p.l.Debug("kafka sync sent",
		logger.String("topic", topic),
		logger.String("key", key),
		logger.Int64("partition", int64(partition)),
		logger.Int64("offset", offset))
	return nil
}

// NoopProducer Kafka 不可用时的空实现，每次返回 error 触发降级
type NoopProducer struct{}

func (p *NoopProducer) ProduceEvent(ctx context.Context, topic string, key string, value []byte) error {
	return fmt.Errorf("kafka unavailable: noop producer")
}

// SaramaAsyncProducer 异步生产者，发送后立即返回，高吞吐
type SaramaAsyncProducer struct {
	producer sarama.AsyncProducer
}

func NewSaramaAsyncProducer(producer sarama.AsyncProducer) Producer {
	return &SaramaAsyncProducer{
		producer: producer,
	}
}

func (p *SaramaAsyncProducer) ProduceEvent(ctx context.Context, topic string, key string, value []byte) error {
	msg := &sarama.ProducerMessage{
		Topic: topic,
		Key:   sarama.StringEncoder(key),
		Value: sarama.ByteEncoder(value),
	}
	// OTel：注入 trace context 到 Kafka header，与 SyncProducer 保持一致
	// 注意：此处 span 只覆盖"入队"耗时，不含 broker ack 时间——异步路径想准确覆盖需要
	// 在 producer.Successes() / Errors() 通道里关联消息元数据延迟 End，代价更高；当前按入队耗时处理
	_, span := saramax.StartProducerSpan(ctx, msg)
	defer span.End()

	select {
	case p.producer.Input() <- msg:
		return nil
	case <-ctx.Done():
		saramax.RecordSpanError(span, ctx.Err())
		return ctx.Err()
	}
}
