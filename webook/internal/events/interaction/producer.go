package interaction

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/webook/internal/events"
)

// InteractionEventProducer 互动事件生产者接口。
// 事件类型由调用方在 InteractionEvent.Type 指定，生产者不写死类型。
type InteractionEventProducer interface {
	Produce(ctx context.Context, evt InteractionEvent) error
}

// SaramaInteractionEventProducer sarama 实现
type SaramaInteractionEventProducer struct {
	producer events.Producer
}

func NewSaramaInteractionEventProducer(producer events.Producer) InteractionEventProducer {
	return &SaramaInteractionEventProducer{producer: producer}
}

// Produce 按 (biz:bizId) 作 key（同一实体有序），JSON 序列化后投递到 TopicInteractionEvents。
func (p *SaramaInteractionEventProducer) Produce(ctx context.Context, evt InteractionEvent) error {
	data, err := json.Marshal(evt)
	if err != nil {
		return err
	}
	key := fmt.Sprintf("%s:%d", evt.Biz, evt.BizId)
	return p.producer.ProduceEvent(ctx, TopicInteractionEvents, key, data)
}
