package interaction

import (
	"context"
	"encoding/json"
	"fmt"

	"gitee.com/train-cloud/geektime-basic-go/internal/events"
)

// InteractionEventProducer 互动事件生产者接口
type InteractionEventProducer interface {
	ProduceReadEvent(ctx context.Context, biz string, bizId int64) error
}

// SaramaInteractionEventProducer sarama 实现
type SaramaInteractionEventProducer struct {
	producer events.Producer
}

func NewSaramaInteractionEventProducer(producer events.Producer) InteractionEventProducer {
	return &SaramaInteractionEventProducer{producer: producer}
}

func (p *SaramaInteractionEventProducer) ProduceReadEvent(ctx context.Context, biz string, bizId int64) error {
	data, err := json.Marshal(InteractionEvent{
		Type:  "read",
		Biz:   biz,
		BizId: bizId,
	})
	if err != nil {
		return err
	}
	key := fmt.Sprintf("%s:%d", biz, bizId)
	return p.producer.ProduceEvent(ctx, TopicInteractionEvents, key, data)
}
