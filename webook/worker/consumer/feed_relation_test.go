package consumer

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/boyxs/train-go/webook/pkg/saramax"

	"github.com/boyxs/train-go/webook/pkg/logger"
	"github.com/boyxs/train-go/webook/worker/consumer/event"
)

// 批内聚合去重：follow/unfollow 失效 follower；block 失效双方；unblock 跳过；一次 InvalidateInboxes。
func TestFeedRelationConsumer_handleBatch_Aggregates(t *testing.T) {
	fake := &fakeFeedClient{}
	c := NewFeedRelationConsumer(nil, fake, saramax.GroupConfig{GroupId: "g"}, logger.NewNopLogger())

	err := c.handleBatch(context.Background(), nil, []event.RelationEvent{
		{Type: event.RelationTypeFollow, FollowerId: 1, FolloweeId: 10},
		{Type: event.RelationTypeUnfollow, FollowerId: 2, FolloweeId: 10},
		{Type: event.RelationTypeBlock, FollowerId: 3, FolloweeId: 4},
		{Type: event.RelationTypeUnblock, FollowerId: 5, FolloweeId: 6}, // 跳过
	})
	require.NoError(t, err)
	require.Len(t, fake.invalidated, 1)
	assert.Equal(t, []int64{1, 2, 3, 4}, fake.invalidated[0], "follower + block 双方，unblock 跳过，顺序去重")
}

// 去重：同一 uid 多次事件只失效一次
func TestFeedRelationConsumer_handleBatch_Dedup(t *testing.T) {
	fake := &fakeFeedClient{}
	c := NewFeedRelationConsumer(nil, fake, saramax.GroupConfig{GroupId: "g"}, logger.NewNopLogger())

	err := c.handleBatch(context.Background(), nil, []event.RelationEvent{
		{Type: event.RelationTypeFollow, FollowerId: 1},
		{Type: event.RelationTypeUnfollow, FollowerId: 1},
	})
	require.NoError(t, err)
	require.Len(t, fake.invalidated, 1)
	assert.Equal(t, []int64{1}, fake.invalidated[0])
}

// 全 unblock：无失效对象，不调 InvalidateInboxes
func TestFeedRelationConsumer_handleBatch_NoOp(t *testing.T) {
	fake := &fakeFeedClient{}
	c := NewFeedRelationConsumer(nil, fake, saramax.GroupConfig{GroupId: "g"}, logger.NewNopLogger())

	err := c.handleBatch(context.Background(), nil, []event.RelationEvent{
		{Type: event.RelationTypeUnblock, FollowerId: 5, FolloweeId: 6},
	})
	require.NoError(t, err)
	assert.Empty(t, fake.invalidated, "无失效对象不发起 RPC")
}
