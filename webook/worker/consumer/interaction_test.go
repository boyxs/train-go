package consumer

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	interactionv1 "github.com/boyxs/train-go/webook/api/gen/interaction/v1"
	"github.com/boyxs/train-go/webook/pkg/logger"
	"github.com/boyxs/train-go/webook/worker/consumer/event"
)

// mockInterClient 桩 interaction gRPC client：只记 BatchIncrReadCount 的入参，其余 RPC 由嵌入接口占位。
type mockInterClient struct {
	interactionv1.InteractionServiceClient
	batchCalls [][]*interactionv1.ReadCountItem
	err        error
}

func (m *mockInterClient) BatchIncrReadCount(_ context.Context, req *interactionv1.BatchIncrReadCountRequest, _ ...grpc.CallOption) (*interactionv1.BatchIncrReadCountResponse, error) {
	m.batchCalls = append(m.batchCalls, req.GetItems())
	if m.err != nil {
		return nil, m.err
	}
	return &interactionv1.BatchIncrReadCountResponse{}, nil
}

// countByBizId 把一次调用的 items 整理成 map[bizId]count，断言时与 items 顺序无关。
func countByBizId(items []*interactionv1.ReadCountItem) map[int64]int64 {
	m := make(map[int64]int64, len(items))
	for _, it := range items {
		m[it.GetBizId()] = it.GetCount()
	}
	return m
}

// 一批多条不同对象 → 一次 BatchIncrReadCount（取代逐条 IncrReadCount 的 N+1）。
func TestHandleBatch_Read_OneBatchCall(t *testing.T) {
	m := &mockInterClient{}
	c := &InteractionConsumer{interClient: m, l: logger.NewNopLogger()}

	err := c.handleBatch(context.Background(), nil, []event.InteractionEvent{
		{Type: "read", Biz: "article", BizId: 123},
		{Type: "read", Biz: "article", BizId: 456},
	})
	require.NoError(t, err)
	require.Len(t, m.batchCalls, 1)
	assert.Equal(t, map[int64]int64{123: 1, 456: 1}, countByBizId(m.batchCalls[0]))
}

// 同一 (biz,bizId) 多次 → 聚合成 count=N，仍只发一次 RPC。
func TestHandleBatch_AggregatesDuplicateBizId(t *testing.T) {
	m := &mockInterClient{}
	c := &InteractionConsumer{interClient: m, l: logger.NewNopLogger()}

	err := c.handleBatch(context.Background(), nil, []event.InteractionEvent{
		{Type: "read", Biz: "article", BizId: 123},
		{Type: "read", Biz: "article", BizId: 123},
		{Type: "read", Biz: "article", BizId: 456},
	})
	require.NoError(t, err)
	require.Len(t, m.batchCalls, 1)
	assert.Equal(t, map[int64]int64{123: 2, 456: 1}, countByBizId(m.batchCalls[0]))
}

// 非 read 类型被忽略，只聚合 read。
func TestHandleBatch_UnknownTypeIgnored(t *testing.T) {
	m := &mockInterClient{}
	c := &InteractionConsumer{interClient: m, l: logger.NewNopLogger()}

	err := c.handleBatch(context.Background(), nil, []event.InteractionEvent{
		{Type: "unknown", Biz: "article", BizId: 1},
		{Type: "read", Biz: "article", BizId: 2},
	})
	require.NoError(t, err)
	require.Len(t, m.batchCalls, 1)
	assert.Equal(t, map[int64]int64{2: 1}, countByBizId(m.batchCalls[0]))
}

// 整批无 read → 不发 RPC（不浪费一次空调用）。
func TestHandleBatch_NoReadEvents_NoCall(t *testing.T) {
	m := &mockInterClient{}
	c := &InteractionConsumer{interClient: m, l: logger.NewNopLogger()}

	err := c.handleBatch(context.Background(), nil, []event.InteractionEvent{
		{Type: "unknown", Biz: "article", BizId: 1},
	})
	require.NoError(t, err)
	assert.Empty(t, m.batchCalls)
}

// 下游报错 → handleBatch 返回 err（saramax 不提交位移，整批重投）。
func TestHandleBatch_ClientError(t *testing.T) {
	m := &mockInterClient{err: errors.New("interaction down")}
	c := &InteractionConsumer{interClient: m, l: logger.NewNopLogger()}

	err := c.handleBatch(context.Background(), nil, []event.InteractionEvent{
		{Type: "read", Biz: "article", BizId: 1},
	})
	assert.Error(t, err)
}
