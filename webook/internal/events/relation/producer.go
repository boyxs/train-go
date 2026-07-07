package relation

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/webook/internal/events"
)

// RelationEventProducer 关系事件生产者接口。事件类型由调用方在 RelationEvent.Type 指定。
type RelationEventProducer interface {
	Produce(ctx context.Context, evt RelationEvent) error
}

// SaramaRelationEventProducer sarama 实现（复用 core 通用 events.Producer）。
type SaramaRelationEventProducer struct {
	producer events.Producer
}

func NewSaramaRelationEventProducer(producer events.Producer) RelationEventProducer {
	return &SaramaRelationEventProducer{producer: producer}
}

// Produce 按 follower_id 作 key（同一关注者有序，利于未来 feed 写扩散按人聚合），JSON 投递 TopicRelationEvents。
func (p *SaramaRelationEventProducer) Produce(ctx context.Context, evt RelationEvent) error {
	data, err := json.Marshal(evt)
	if err != nil {
		return err
	}
	key := strconv.FormatInt(evt.FollowerId, 10)
	return p.producer.ProduceEvent(ctx, TopicRelationEvents, key, data)
}
