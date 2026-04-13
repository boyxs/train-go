package events

import (
	"context"
	"fmt"

	"gitee.com/train-cloud/geektime-basic-go/pkg/logger"
	"github.com/IBM/sarama"
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
	partition, offset, err := p.producer.SendMessage(&sarama.ProducerMessage{
		Topic: topic,
		Key:   sarama.StringEncoder(key),
		Value: sarama.ByteEncoder(value),
	})
	if err != nil {
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
	select {
	case p.producer.Input() <- &sarama.ProducerMessage{
		Topic: topic,
		Key:   sarama.StringEncoder(key),
		Value: sarama.ByteEncoder(value),
	}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
