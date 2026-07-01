package interaction

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockProducer struct {
	lastTopic string
	lastKey   string
	lastValue []byte
	err       error
}

func (m *mockProducer) ProduceEvent(_ context.Context, topic string, key string, value []byte) error {
	m.lastTopic = topic
	m.lastKey = key
	m.lastValue = value
	return m.err
}

func TestProduce(t *testing.T) {
	mock := &mockProducer{}
	p := NewSaramaInteractionEventProducer(mock)

	err := p.Produce(context.Background(), InteractionEvent{Type: TypeRead, Biz: "article", BizId: 123})
	require.NoError(t, err)

	assert.Equal(t, TopicInteractionEvents, mock.lastTopic)
	assert.Equal(t, "article:123", mock.lastKey)

	var evt InteractionEvent
	err = json.Unmarshal(mock.lastValue, &evt)
	require.NoError(t, err)
	assert.Equal(t, TypeRead, evt.Type)
	assert.Equal(t, "article", evt.Biz)
	assert.Equal(t, int64(123), evt.BizId)
}

func TestProduceEvent_Error(t *testing.T) {
	mock := &mockProducer{err: errors.New("kafka unavailable")}
	p := NewSaramaInteractionEventProducer(mock)

	err := p.Produce(context.Background(), InteractionEvent{Type: TypeRead, Biz: "article", BizId: 123})
	assert.ErrorContains(t, err, "kafka unavailable")
}
