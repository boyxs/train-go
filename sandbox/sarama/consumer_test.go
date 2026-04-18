package kafka

import (
	"context"
	"log"
	"testing"
	"time"

	"github.com/IBM/sarama"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

func TestConsumerGroup(t *testing.T) {
	cfg := sarama.NewConfig()
	cfg.Consumer.Offsets.Initial = sarama.OffsetOldest

	consumer, err := sarama.NewConsumerGroup(addrs, "test-group", cfg)
	require.NoError(t, err)
	defer consumer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	start := time.Now()
	// Consume 完成一轮 rebalance 就会返回，必须循环调用才能持续消费
	for {
		err = consumer.Consume(ctx, []string{"test_topic"}, &ConsumerGroupHandler{t: t})
		if err != nil {
			t.Logf("消费出错: %v", err)
		}
		if ctx.Err() != nil {
			break
		}
	}

	t.Logf("消费结束，耗时 %s", time.Since(start))
}

// ConsumerGroupHandler 实现 sarama.ConsumerGroupHandler 接口
type ConsumerGroupHandler struct {
	t *testing.T
}

func (h *ConsumerGroupHandler) Setup(_ sarama.ConsumerGroupSession) error   { return nil }
func (h *ConsumerGroupHandler) Cleanup(_ sarama.ConsumerGroupSession) error { return nil }

func (h *ConsumerGroupHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	msgs := claim.Messages
	const batchSize = 10

	for {
		batch := make([]*sarama.ConsumerMessage, 0, batchSize)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		var done = false
		var eg errgroup.Group
		for i := 0; i < batchSize && !done; i++ {
			select {
			case <-ctx.Done():
				done = true
			case msg, ok := <-msgs():
				if !ok {
					cancel()
					return nil
				}
				batch = append(batch, msg)
				eg.Go(func() error {
					return nil
				})
			}
		}
		cancel()
		err := eg.Wait()
		if err != nil {
			log.Println(err)
			continue
		}

		for _, msg := range batch {
			h.t.Logf("收到消息: topic=%s partition=%d offset=%d key=%s value=%s",
				msg.Topic, msg.Partition, msg.Offset, string(msg.Key), string(msg.Value))
			// 标记已消费，提交 offset
			session.MarkMessage(msg, "")
		}

		println()
	}
}

func (h *ConsumerGroupHandler) ConsumeClaimV1(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for msg := range claim.Messages() {
		h.t.Logf("收到消息: topic=%s partition=%d offset=%d key=%s value=%s",
			msg.Topic, msg.Partition, msg.Offset, string(msg.Key), string(msg.Value))
		// 标记已消费，提交 offset
		session.MarkMessage(msg, "")
	}
	return nil
}
