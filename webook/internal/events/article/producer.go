package article

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/boyxs/train-go/webook/internal/events"
)

// ArticleEventProducer 文章事件生产者接口。事件类型由调用方在 ArticleEvent.Type 指定。
type ArticleEventProducer interface {
	Produce(ctx context.Context, evt ArticleEvent) error
}

// SaramaArticleEventProducer sarama 实现（复用 core 通用 events.Producer）。
type SaramaArticleEventProducer struct {
	producer events.Producer
}

func NewSaramaArticleEventProducer(producer events.Producer) ArticleEventProducer {
	return &SaramaArticleEventProducer{producer: producer}
}

// Produce 按 author_id 作 key（同一作者 publish→withdraw 有序），JSON 投递 TopicArticleEvents。
func (p *SaramaArticleEventProducer) Produce(ctx context.Context, evt ArticleEvent) error {
	data, err := json.Marshal(evt)
	if err != nil {
		return err
	}
	key := strconv.FormatInt(evt.AuthorId, 10)
	return p.producer.ProduceEvent(ctx, TopicArticleEvents, key, data)
}
