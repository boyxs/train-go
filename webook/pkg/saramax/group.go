package saramax

import (
	"context"
	"time"

	"github.com/IBM/sarama"

	"github.com/boyxs/train-go/webook/pkg/logger"
)

// GroupConfig 消费组连接 + 退避重连参数。
type GroupConfig struct {
	Addrs          []string
	GroupId        string
	BackoffInitial time.Duration
	BackoffMax     time.Duration
}

// RunGroup 自管连接 + 无限退避重连消费 topics，直到 ctx 取消。
// 供各服务的消费者复用同一调度骨架：连不上 Kafka 时后台退避重试（不阻塞启动），
// Consume 出错重连；ctx 取消即优雅退出。name 仅用于日志区分消费者。
func RunGroup(ctx context.Context, cfg GroupConfig, saramaCfg *sarama.Config, l logger.LoggerX,
	name string, topics []string, handler sarama.ConsumerGroupHandler) error {
	initial := cfg.BackoffInitial
	if initial <= 0 {
		initial = time.Second
	}
	maxBackoff := cfg.BackoffMax
	if maxBackoff < initial {
		maxBackoff = initial // BackoffMax 未配置(0)/过小 → 不小于 initial，防 growBackoff 归零致紧忙重连
	}
	backoff := initial
	for ctx.Err() == nil {
		group, err := sarama.NewConsumerGroup(cfg.Addrs, cfg.GroupId, saramaCfg)
		if err != nil {
			l.Warn(ctx, "连接 Kafka 失败，后台重试",
				logger.String("consumer", name), logger.String("backoff", backoff.String()), logger.Error(err))
			if !backoffSleep(ctx, backoff) {
				return nil
			}
			backoff = growBackoff(backoff, maxBackoff) // 增长退避，上限 maxBackoff
			continue
		}
		backoff = initial // 重连成功，退避归位到（已兜底的）初始值
		for ctx.Err() == nil {
			if err = group.Consume(ctx, topics, handler); err != nil {
				l.Warn(ctx, "消费出错，重连", logger.String("consumer", name), logger.Error(err))
				break
			}
		}
		if closeErr := group.Close(); closeErr != nil {
			l.Warn(ctx, "关闭消费者组出错", logger.String("consumer", name), logger.Error(closeErr))
		}
	}
	return nil
}

func backoffSleep(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}

func growBackoff(d, upper time.Duration) time.Duration {
	if d *= 2; d > upper {
		return upper
	}
	return d
}
