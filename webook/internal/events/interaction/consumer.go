package interaction

import (
	"context"
	"encoding/json"
	"time"

	"gitee.com/train-cloud/geektime-basic-go/internal/events"
	"gitee.com/train-cloud/geektime-basic-go/internal/repository"
	"gitee.com/train-cloud/geektime-basic-go/pkg/logger"
	"github.com/IBM/sarama"
	"golang.org/x/sync/errgroup"
)

// ConsumerConfig Consumer 配置
type ConsumerConfig struct {
	GroupID         string
	BackoffInitial  time.Duration
	BackoffMax      time.Duration
}

// SaramaInteractionEventConsumer 互动事件消费者 sarama 实现
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
	// 连接失败退避：从 BackoffInitial 起，指数增长到 BackoffMax
	backoff := c.cfg.BackoffInitial
	maxBackoff := c.cfg.BackoffMax
	for {
		err = group.Consume(ctx, []string{TopicInteractionEvents}, c)
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
		// 消费成功，重置退避
		backoff = c.cfg.BackoffInitial
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
}

func (c *SaramaInteractionEventConsumer) Setup(_ sarama.ConsumerGroupSession) error   { return nil }
func (c *SaramaInteractionEventConsumer) Cleanup(_ sarama.ConsumerGroupSession) error { return nil }

func (c *SaramaInteractionEventConsumer) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	const batchSize = 10
	msgCh := claim.Messages()

	for {
		batch := make([]*sarama.ConsumerMessage, 0, batchSize)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)

		for i := 0; i < batchSize; i++ {
			select {
			case <-ctx.Done():
				goto process
			case msg, ok := <-msgCh:
				if !ok {
					cancel()
					c.processBatch(session, batch)
					return nil
				}
				batch = append(batch, msg)
			}
		}

	process:
		cancel()
		if len(batch) == 0 {
			continue
		}
		c.processBatch(session, batch)
	}
}

func (c *SaramaInteractionEventConsumer) processBatch(session sarama.ConsumerGroupSession, batch []*sarama.ConsumerMessage) {
	var eg errgroup.Group
	for _, msg := range batch {
		eg.Go(func() error {
			var evt InteractionEvent
			if err := json.Unmarshal(msg.Value, &evt); err != nil {
				return err
			}
			switch evt.Type {
			case "read":
				return c.repo.IncrReadCount(context.Background(), evt.Biz, evt.BizId)
			default:
				return nil
			}
		})
	}
	if err := eg.Wait(); err != nil {
		c.l.Error("批量处理互动事件失败", logger.Error(err))
		return
	}
	for _, msg := range batch {
		session.MarkMessage(msg, "")
	}
}
