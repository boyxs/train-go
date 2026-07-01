package events

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/IBM/sarama"

	"github.com/webook/pkg/logger"
	"github.com/webook/pkg/saramax"
)

// SaramaSyncProducer 同步生产者，发送后等待 broker 确认
type SaramaSyncProducer struct {
	producer sarama.SyncProducer
	l        logger.LoggerX
}

func (p *SaramaSyncProducer) ProduceEvent(ctx context.Context, topic string, key string, value []byte) error {
	msg := &sarama.ProducerMessage{
		Topic: topic,
		Key:   sarama.StringEncoder(key),
		Value: sarama.ByteEncoder(value),
	}
	// OTel：创建 Producer span + 把 trace context 注入 Kafka headers，
	// consumer 端 Extract 后可挂上同一 trace
	_, span := saramax.StartProducerSpan(ctx, msg)
	defer span.End()

	partition, offset, err := p.producer.SendMessage(msg)
	if err != nil {
		saramax.RecordSpanError(span, err)
		return fmt.Errorf("kafka sync send failed: topic=%s key=%s err=%w", topic, key, err)
	}
	p.l.Debug("kafka sync sent",
		logger.String("topic", topic),
		logger.String("key", key),
		logger.Int64("partition", int64(partition)),
		logger.Int64("offset", offset))
	return nil
}

// LazyProducer 懒连接生产者：构造不阻塞，后台无限退避连 Kafka，启动不依赖 Kafka 就绪。
// 连上前 ProduceEvent 返回 error（触发上层熔断降级为同步兜底），连上后委托 SaramaSyncProducer。
type LazyProducer struct {
	connect     func() (Producer, error) // 可注入，默认 sarama dial；单测换桩
	l           logger.LoggerX
	backoffInit time.Duration
	backoffMax  time.Duration
	delegate    atomic.Pointer[producerBox]
}

// producerBox 包一层让 atomic.Pointer 能存接口值。
type producerBox struct{ p Producer }

// NewLazyProducer 立即返回，连接在后台进行。
func NewLazyProducer(addrs []string, cfg *sarama.Config, l logger.LoggerX,
	backoffInit, backoffMax time.Duration) Producer {
	return newLazyProducer(l, backoffInit, backoffMax, func() (Producer, error) {
		sp, err := sarama.NewSyncProducer(addrs, cfg)
		if err != nil {
			return nil, err
		}
		return &SaramaSyncProducer{producer: sp, l: l}, nil
	})
}

// newLazyProducer connect 可注入，便于单测不真实拨号。
func newLazyProducer(l logger.LoggerX, backoffInit, backoffMax time.Duration,
	connect func() (Producer, error)) *LazyProducer {
	p := &LazyProducer{
		connect:     connect,
		l:           l,
		backoffInit: backoffInit,
		backoffMax:  backoffMax,
	}
	go p.run()
	return p
}

// run 后台无限退避连接，成功即退出（之后由 sarama 内部维护重连）。
func (p *LazyProducer) run() {
	backoff, backoffMax := p.backoffInit, p.backoffMax
	if backoff <= 0 {
		backoff = time.Second
	}
	if backoffMax < backoff {
		backoffMax = 30 * time.Second
	}
	for {
		d, err := p.connect()
		if err == nil {
			p.delegate.Store(&producerBox{p: d})
			p.l.Info("kafka producer 连接成功")
			return
		}
		p.l.Warn("kafka producer 连接失败，后台重试",
			logger.String("backoff", backoff.String()), logger.Error(err))
		time.Sleep(backoff)
		if backoff *= 2; backoff > backoffMax {
			backoff = backoffMax
		}
	}
}

func (p *LazyProducer) ProduceEvent(ctx context.Context, topic string, key string, value []byte) error {
	box := p.delegate.Load()
	if box == nil {
		return fmt.Errorf("kafka producer 连接中，暂不可用")
	}
	return box.p.ProduceEvent(ctx, topic, key, value)
}
