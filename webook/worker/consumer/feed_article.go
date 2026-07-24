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

// FeedArticleConsumer 消费 article_events → 调 feed gRPC 写扩散/移除（调度器只派发，不持数据）。
type FeedArticleConsumer struct {
	saramaCfg  *sarama.Config
	feedClient feedv1.FeedServiceClient
	cfg        saramax.GroupConfig
	l          logger.LoggerX
}

func NewFeedArticleConsumer(saramaCfg *sarama.Config, feedClient feedv1.FeedServiceClient, cfg saramax.GroupConfig, l logger.LoggerX) *FeedArticleConsumer {
	cfg.GroupId += "-feed-article" // 独立 group，避免与 interaction / feed-relation 竞争分区
	return &FeedArticleConsumer{saramaCfg: saramaCfg, feedClient: feedClient, cfg: cfg, l: l}
}

func (c *FeedArticleConsumer) Start(ctx context.Context) error {
	handler := saramax.NewBatchConsumer[event.ArticleEvent](c.handleBatch, 10, time.Second, c.l)
	return saramax.RunGroup(ctx, c.cfg, c.saramaCfg, c.l, "feed-article",
		[]string{event.TopicArticleEvents}, handler)
}

// handleBatch 逐事件派发（key=authorId 保证同作者 publish→withdraw 有序）：published→扩散，withdrawn→移除。
// 某条失败整批重投——feed 侧 ZADD/DEL 幂等，重放安全。
func (c *FeedArticleConsumer) handleBatch(ctx context.Context, _ []*sarama.ConsumerMessage, evts []event.ArticleEvent) error {
	for _, e := range evts {
		switch e.Type {
		case event.ArticleTypePublished:
			if _, err := c.feedClient.FanoutArticle(ctx, &feedv1.FanoutArticleRequest{
				ArticleId: e.ArticleId, AuthorId: e.AuthorId, PublishedAt: e.Ts,
			}); err != nil {
				return err
			}
		case event.ArticleTypeWithdrawn:
			if _, err := c.feedClient.RemoveArticle(ctx, &feedv1.RemoveArticleRequest{
				ArticleId: e.ArticleId, AuthorId: e.AuthorId,
			}); err != nil {
				return err
			}
		}
	}
	return nil
}
