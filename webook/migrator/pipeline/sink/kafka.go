package sink

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/IBM/sarama"

	"github.com/boyxs/train-go/webook/pkg/logger"
)

// KafkaSink 把 Mutation 发到 Kafka topic（下游消费者订阅 binlog 风格变更流）。
//
// 设计：
//   - Op + Table + PK + Cols + Version 序列化 JSON 作为消息 value
//   - Message key = strconv.FormatInt(PK, 10) → 同 PK 落同 partition（保单行有序）
//   - 使用 SyncProducer（每次 Apply 同步刷 batch）；失败上抛由调用方重试 / dead_letter
type KafkaSink struct {
	producer sarama.SyncProducer
	topic    string
	l        logger.LoggerX
}

// kafkaPayload 是 Kafka 消息 value 的 JSON schema。
type kafkaPayload struct {
	Op      string         `json:"op"`
	Table   string         `json:"table"`
	PK      string         `json:"pk"`
	Cols    map[string]any `json:"cols,omitempty"`
	Version int64          `json:"version,omitempty"`
}

func NewKafkaSink(producer sarama.SyncProducer, topic string, l logger.LoggerX) Sink {
	return &KafkaSink{producer: producer, topic: topic, l: l}
}

func (s *KafkaSink) Apply(_ context.Context, batch []Mutation) error {
	if len(batch) == 0 {
		return nil
	}
	msgs := make([]*sarama.ProducerMessage, 0, len(batch))
	for _, m := range batch {
		body, err := json.Marshal(kafkaPayload(m))
		if err != nil {
			return fmt.Errorf("marshal kafka payload pk=%s: %w", m.PK, err)
		}
		msgs = append(msgs, &sarama.ProducerMessage{
			Topic: s.topic,
			Key:   sarama.StringEncoder(m.PK),
			Value: sarama.ByteEncoder(body),
		})
	}
	if err := s.producer.SendMessages(msgs); err != nil {
		return fmt.Errorf("kafka send messages: %w", err)
	}
	return nil
}

func (s *KafkaSink) Close() error {
	return s.producer.Close()
}
