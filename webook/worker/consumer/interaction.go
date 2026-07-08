package consumer

import (
	"context"
	"time"

	"github.com/IBM/sarama"

	interactionv1 "github.com/boyxs/train-go/webook/api/gen/interaction/v1"
	"github.com/boyxs/train-go/webook/pkg/logger"
	"github.com/boyxs/train-go/webook/pkg/saramax"
	"github.com/boyxs/train-go/webook/worker/consumer/event"
)

// ConsumerConfig 互动事件消费配置。
type ConsumerConfig struct {
	Addrs          []string
	GroupID        string
	BackoffInitial time.Duration
	BackoffMax     time.Duration
}

// InteractionConsumer 消费 read 事件 → 调 interaction gRPC 累加阅读数（调度器只派发，不持数据）。
// 自管连接：Start 内无限退避重连，启动不依赖 Kafka 实例就绪。
type InteractionConsumer struct {
	saramaCfg   *sarama.Config
	interClient interactionv1.InteractionServiceClient
	cfg         ConsumerConfig
	l           logger.LoggerX
}

func NewInteractionConsumer(
	saramaCfg *sarama.Config,
	interClient interactionv1.InteractionServiceClient,
	cfg ConsumerConfig,
	l logger.LoggerX,
) *InteractionConsumer {
	return &InteractionConsumer{saramaCfg: saramaCfg, interClient: interClient, cfg: cfg, l: l}
}

func (c *InteractionConsumer) Start(ctx context.Context) error {
	handler := saramax.NewBatchConsumer[event.InteractionEvent](c.handleBatch, 10, time.Second, c.l)
	backoff := c.cfg.BackoffInitial
	if backoff <= 0 {
		backoff = time.Second
	}
	for ctx.Err() == nil {
		group, err := sarama.NewConsumerGroup(c.cfg.Addrs, c.cfg.GroupID, c.saramaCfg)
		if err != nil {
			c.l.Warn("连接 Kafka 失败，后台重试",
				logger.String("backoff", backoff.String()), logger.Error(err))
			if !sleep(ctx, backoff) {
				return nil
			}
			backoff = grow(backoff, c.cfg.BackoffMax)
			continue
		}
		backoff = c.cfg.BackoffInitial
		for ctx.Err() == nil {
			if err = group.Consume(ctx, []string{event.TopicInteractionEvents}, handler); err != nil {
				c.l.Warn("消费互动事件出错，重连", logger.Error(err))
				break
			}
		}
		if closeErr := group.Close(); closeErr != nil {
			c.l.Warn("关闭消费者组出错", logger.Error(closeErr))
		}
	}
	return nil
}

// handleBatch ctx 已带 Kafka header 里的 trace context，下游 gRPC span 自动挂上。
// 按 (biz,biz_id) 聚合本批 read 次数，一批一次 BatchIncrReadCount：取代逐条 IncrReadCount 的
// N+1 over-the-wire；整批一次提交——某条失败时整批重投，不会重复累加批内已成功项。
func (c *InteractionConsumer) handleBatch(ctx context.Context, _ []*sarama.ConsumerMessage, evts []event.InteractionEvent) error {
	type aggKey struct {
		biz   string
		bizId int64
	}
	counts := make(map[aggKey]int64, len(evts))
	order := make([]aggKey, 0, len(evts))
	for _, evt := range evts {
		if evt.Type != "read" {
			continue
		}
		k := aggKey{biz: evt.Biz, bizId: evt.BizId}
		if _, ok := counts[k]; !ok {
			order = append(order, k)
		}
		counts[k]++
	}
	if len(order) == 0 {
		return nil
	}
	items := make([]*interactionv1.ReadCountItem, 0, len(order))
	for _, k := range order {
		items = append(items, &interactionv1.ReadCountItem{Biz: k.biz, BizId: k.bizId, Count: counts[k]})
	}
	_, err := c.interClient.BatchIncrReadCount(ctx, &interactionv1.BatchIncrReadCountRequest{Items: items})
	return err
}

func sleep(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}

func grow(d, max time.Duration) time.Duration {
	if d *= 2; d > max {
		return max
	}
	return d
}
