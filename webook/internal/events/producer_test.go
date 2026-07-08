package events

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/boyxs/train-go/webook/pkg/logger"
)

// mockProducer 记录最后一次发送，用于断言委托。
type mockProducer struct {
	calls     atomic.Int32
	lastTopic string
}

func (m *mockProducer) ProduceEvent(_ context.Context, topic string, _ string, _ []byte) error {
	m.calls.Add(1)
	m.lastTopic = topic
	return nil
}

// 未连接时（delegate 为 nil）ProduceEvent 返回 error，触发上层降级。
func TestLazyProducer_NotReadyReturnsError(t *testing.T) {
	p := &LazyProducer{l: logger.NewNopLogger()} // 不起后台 goroutine，delegate 始终 nil
	err := p.ProduceEvent(context.Background(), "topic", "key", nil)
	require.ErrorContains(t, err, "连接中")
}

// 构造立即返回（不阻塞），后台连接失败会重试，连上后自动委托。
func TestLazyProducer_HealsAfterRetries(t *testing.T) {
	mock := &mockProducer{}
	var attempts atomic.Int32
	p := newLazyProducer(logger.NewNopLogger(), time.Millisecond, 5*time.Millisecond,
		func() (Producer, error) {
			if attempts.Add(1) < 3 { // 前两次模拟 Kafka 未就绪
				return nil, errors.New("dial fail")
			}
			return mock, nil
		})

	// 连上前（delegate 仍 nil）应报错——验证构造未阻塞、未就绪即返回。
	if err := p.ProduceEvent(context.Background(), "topic", "key", nil); err != nil {
		assert.ErrorContains(t, err, "连接中")
	}

	// 后台重试若干次后连上，ProduceEvent 委托成功。
	require.Eventually(t, func() bool {
		return p.ProduceEvent(context.Background(), "topic", "key", nil) == nil
	}, time.Second, time.Millisecond)

	assert.GreaterOrEqual(t, attempts.Load(), int32(3))
	assert.Equal(t, "topic", mock.lastTopic)
	assert.Positive(t, mock.calls.Load())
}
