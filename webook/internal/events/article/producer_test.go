package article_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/boyxs/train-go/webook/internal/events/article"
)

// fakeProducer 捕获底层 ProduceEvent 的入参，用于断言 topic/key/JSON 契约。
type fakeProducer struct {
	topic string
	key   string
	value []byte
}

func (f *fakeProducer) ProduceEvent(_ context.Context, topic, key string, value []byte) error {
	f.topic, f.key, f.value = topic, key, value
	return nil
}

// 冻结契约：topic=article_events，key=authorId，JSON 字段名/取值稳定（与 worker 消费副本对齐）。
func TestSaramaArticleEventProducer_Produce(t *testing.T) {
	fp := &fakeProducer{}
	p := article.NewSaramaArticleEventProducer(fp)

	err := p.Produce(context.Background(), article.ArticleEvent{
		Type: article.TypePublished, ArticleId: 1001, AuthorId: 7, Ts: 1770000000000,
	})
	require.NoError(t, err)

	assert.Equal(t, article.TopicArticleEvents, fp.topic)
	assert.Equal(t, "7", fp.key, "按 authorId 分区，保证同作者事件有序")
	assert.JSONEq(t, `{"type":"published","articleId":1001,"authorId":7,"ts":1770000000000}`, string(fp.value))
}
