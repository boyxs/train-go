package interaction

import (
	"context"
	"encoding/json"
	"testing"

	"gitee.com/train-cloud/geektime-basic-go/internal/domain"
	"gitee.com/train-cloud/geektime-basic-go/pkg/logger"
	"github.com/IBM/sarama"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// mockSession 仅实现 MarkMessage，其他方法不会被调用
type mockSession struct {
	sarama.ConsumerGroupSession
	marked []*sarama.ConsumerMessage
}

func (m *mockSession) MarkMessage(msg *sarama.ConsumerMessage, _ string) {
	m.marked = append(m.marked, msg)
}

func buildMsg(t *testing.T, evt InteractionEvent) *sarama.ConsumerMessage {
	data, err := json.Marshal(evt)
	require.NoError(t, err)
	return &sarama.ConsumerMessage{Value: data}
}

func TestProcessBatch_Read(t *testing.T) {
	r := &mockRepo{}
	c := &SaramaInteractionEventConsumer{repo: r, l: logger.NewNopLogger()}
	sess := &mockSession{}

	c.processBatch(sess, []*sarama.ConsumerMessage{
		buildMsg(t, InteractionEvent{Type: "read", Biz: "article", BizId: 123}),
	})
	require.Len(t, r.readCalls, 1)
	assert.Equal(t, "article", r.readCalls[0].biz)
	assert.Equal(t, int64(123), r.readCalls[0].bizId)
	assert.Len(t, sess.marked, 1)
}

func TestProcessBatch_UnknownType(t *testing.T) {
	r := &mockRepo{}
	c := &SaramaInteractionEventConsumer{repo: r, l: logger.NewNopLogger()}
	sess := &mockSession{}

	c.processBatch(sess, []*sarama.ConsumerMessage{
		buildMsg(t, InteractionEvent{Type: "unknown", Biz: "article", BizId: 123}),
	})
	assert.Empty(t, r.readCalls)
	assert.Len(t, sess.marked, 1)
}

func TestProcessBatch_InvalidJSON(t *testing.T) {
	r := &mockRepo{}
	c := &SaramaInteractionEventConsumer{repo: r, l: logger.NewNopLogger()}
	sess := &mockSession{}

	c.processBatch(sess, []*sarama.ConsumerMessage{
		{Value: []byte("not json")},
	})
	// 解析失败不提交 offset
	assert.Empty(t, sess.marked)
}
