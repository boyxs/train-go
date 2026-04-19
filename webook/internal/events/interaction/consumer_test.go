package interaction

import (
	"context"
	"errors"
	"testing"

	"github.com/IBM/sarama"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/webook/internal/domain"
	"github.com/webook/pkg/logger"
)

type mockRepo struct {
	readCalls []readCall
	err       error
}

type readCall struct {
	biz   string
	bizId int64
}

func (m *mockRepo) IncrReadCount(_ context.Context, biz string, bizId int64) error {
	m.readCalls = append(m.readCalls, readCall{biz, bizId})
	return m.err
}

// 以下方法不在 Consumer 中使用，留空实现满足接口
func (m *mockRepo) Like(context.Context, int64, string, int64) error          { return nil }
func (m *mockRepo) CancelLike(context.Context, int64, string, int64) error    { return nil }
func (m *mockRepo) Collect(context.Context, int64, string, int64) error       { return nil }
func (m *mockRepo) CancelCollect(context.Context, int64, string, int64) error { return nil }
func (m *mockRepo) FindInteraction(context.Context, int64, string, int64) (domain.Interaction, error) {
	return domain.Interaction{}, nil
}
func (m *mockRepo) FindUserState(context.Context, int64, string, int64) (bool, bool, error) {
	return false, false, nil
}
func (m *mockRepo) FindByBizIds(context.Context, string, []int64) ([]domain.Interaction, error) {
	return nil, nil
}
func (m *mockRepo) ListCollectedBizIds(context.Context, int64, string, int) ([]int64, error) {
	return nil, nil
}
func (m *mockRepo) ListHotBizIds(context.Context, string, int) ([]int64, error) { return nil, nil }

func TestHandleBatch_Read(t *testing.T) {
	r := &mockRepo{}
	c := &SaramaInteractionEventConsumer{repo: r, l: logger.NewNopLogger()}

	err := c.handleBatch(context.Background(), nil, []InteractionEvent{
		{Type: "read", Biz: "article", BizId: 123},
		{Type: "read", Biz: "article", BizId: 456},
	})
	require.NoError(t, err)
	require.Len(t, r.readCalls, 2)
	assert.Equal(t, int64(123), r.readCalls[0].bizId)
	assert.Equal(t, int64(456), r.readCalls[1].bizId)
}

func TestHandleBatch_UnknownTypeIgnored(t *testing.T) {
	r := &mockRepo{}
	c := &SaramaInteractionEventConsumer{repo: r, l: logger.NewNopLogger()}

	err := c.handleBatch(context.Background(), nil, []InteractionEvent{
		{Type: "unknown", Biz: "article", BizId: 1},
		{Type: "read", Biz: "article", BizId: 2},
	})
	require.NoError(t, err)
	require.Len(t, r.readCalls, 1)
	assert.Equal(t, int64(2), r.readCalls[0].bizId)
}

func TestHandleBatch_RepoError(t *testing.T) {
	r := &mockRepo{err: errors.New("db down")}
	c := &SaramaInteractionEventConsumer{repo: r, l: logger.NewNopLogger()}

	err := c.handleBatch(context.Background(), nil, []InteractionEvent{
		{Type: "read", Biz: "article", BizId: 1},
	})
	assert.Error(t, err)
}

// 防止 sarama 类型未使用引发 import 警告
var _ = sarama.ConsumerMessage{}
