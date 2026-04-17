package interaction

import (
	"context"
	"time"

	"github.com/webook/internal/events"
	"github.com/webook/internal/repository"
	"github.com/webook/pkg/logger"
	"github.com/webook/pkg/saramax"
	"github.com/IBM/sarama"
)

// ConsumerConfig Consumer 配置
type ConsumerConfig struct {
	GroupID        string
	BackoffInitial time.Duration
	BackoffMax     time.Duration
}

// SaramaInteractionEventConsumer 互动事件消费者，业务只关心 handleBatch 业务逻辑
// 攒批 / 反序列化 / 提交 offset 全部由 saramax.BatchConsumer 负责
type SaramaInteractionEventConsumer struct {
	client sarama.Client
	repo   repository.InteractionRepository
	cfg    ConsumerConfig
	l      logger.LoggerX
}

func NewSaramaInteractionEventConsumer(
	client sarama.Client,
	repo repository.InteractionRepository,
	cfg ConsumerConfig,
	l logger.LoggerX,
) events.Consumer {
	return &SaramaInteractionEventConsumer{client: client, repo: repo, cfg: cfg, l: l}
}

func (c *SaramaInteractionEventConsumer) Start(ctx context.Context) error {
	if c.client == nil {
		c.l.Info("kafka client 未初始化，consumer 不启动")
		return nil
	}
	group, err := sarama.NewConsumerGroupFromClient(c.cfg.GroupID, c.client)
	if err != nil {
		return err
	}
	defer group.Close()

	// 用通用批量消费者：业务只写 handleBatch
	handler := saramax.NewBatchConsumer[InteractionEvent](
		c.handleBatch, 10, time.Second, c.l,
	)

	// 连接失败退避：从 BackoffInitial 起，指数增长到 BackoffMax
	backoff := c.cfg.BackoffInitial
	maxBackoff := c.cfg.BackoffMax
	for {
		err = group.Consume(ctx, []string{TopicInteractionEvents}, handler)
		if err != nil {
			c.l.Warn("消费互动事件出错，稍后重试",
				logger.String("backoff", backoff.String()),
				logger.Error(err))
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}
		backoff = c.cfg.BackoffInitial
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
}

// handleBatch 业务逻辑：批量处理互动事件
func (c *SaramaInteractionEventConsumer) handleBatch(_ []*sarama.ConsumerMessage, events []InteractionEvent) error {
	ctx := context.Background()
	for _, evt := range events {
		switch evt.Type {
		case "read":
			if err := c.repo.IncrReadCount(ctx, evt.Biz, evt.BizId); err != nil {
				return err
			}
		}
	}
	return nil
}
