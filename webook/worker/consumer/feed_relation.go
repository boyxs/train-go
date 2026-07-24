package consumer

import (
	"context"
	"time"

	"github.com/IBM/sarama"

	feedv1 "github.com/boyxs/train-go/webook/api/gen/feed/v1"
	"github.com/boyxs/train-go/webook/pkg/logger"
	"github.com/boyxs/train-go/webook/pkg/saramax"
	"github.com/boyxs/train-go/webook/worker/consumer/event"
)

// FeedRelationConsumer 消费 relation_events → 调 feed gRPC 失效重建（失效对象按事件类型聚合去重）。
type FeedRelationConsumer struct {
	saramaCfg  *sarama.Config
	feedClient feedv1.FeedServiceClient
	cfg        saramax.GroupConfig
	l          logger.LoggerX
}

func NewFeedRelationConsumer(saramaCfg *sarama.Config, feedClient feedv1.FeedServiceClient, cfg saramax.GroupConfig, l logger.LoggerX) *FeedRelationConsumer {
	cfg.GroupId += "-feed-relation" // 独立 group
	return &FeedRelationConsumer{saramaCfg: saramaCfg, feedClient: feedClient, cfg: cfg, l: l}
}

func (c *FeedRelationConsumer) Start(ctx context.Context) error {
	handler := saramax.NewBatchConsumer[event.RelationEvent](c.handleBatch, 10, time.Second, c.l)
	return saramax.RunGroup(ctx, c.cfg, c.saramaCfg, c.l, "feed-relation",
		[]string{event.TopicRelationEvents}, handler)
}

// handleBatch 批内聚合去重后一次 InvalidateInboxes：
// follow/unfollow→失效 follower；block→失效双方（级联解除双向关注）；unblock→跳过（不恢复关注）。
func (c *FeedRelationConsumer) handleBatch(ctx context.Context, _ []*sarama.ConsumerMessage, evts []event.RelationEvent) error {
	seen := make(map[int64]struct{}, len(evts))
	uids := make([]int64, 0, len(evts))
	add := func(uid int64) {
		if uid <= 0 {
			return
		}
		if _, ok := seen[uid]; !ok {
			seen[uid] = struct{}{}
			uids = append(uids, uid)
		}
	}
	for _, e := range evts {
		switch e.Type {
		case event.RelationTypeFollow, event.RelationTypeUnfollow:
			add(e.FollowerId)
		case event.RelationTypeBlock:
			add(e.FollowerId)
			add(e.FolloweeId)
		}
	}
	if len(uids) == 0 {
		return nil
	}
	_, err := c.feedClient.InvalidateInboxes(ctx, &feedv1.InvalidateInboxesRequest{Uids: uids})
	return err
}
