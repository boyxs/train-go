package service

import (
	"context"

	intrevt "github.com/boyxs/train-go/webook/internal/events/interaction"
	"github.com/boyxs/train-go/webook/pkg/circuitbreaker"
	"github.com/boyxs/train-go/webook/pkg/logger"
)

// KafkaInteractionService 装饰器
// IncrReadCount 走 Kafka 异步（高频、无用户状态、允许延迟）
// Like/Collect 等走内层同步（用户期望立刻看到状态变化）
// 内置熔断：连续失败后跳过 Kafka，冷却后自动恢复
type KafkaInteractionService struct {
	InteractionService
	producer intrevt.InteractionEventProducer
	breaker  circuitbreaker.CircuitBreaker
	l        logger.LoggerX
}

func NewKafkaInteractionService(
	svc InteractionService,
	producer intrevt.InteractionEventProducer,
	breaker circuitbreaker.CircuitBreaker,
	l logger.LoggerX,
) InteractionService {
	return &KafkaInteractionService{
		InteractionService: svc,
		producer:           producer,
		breaker:            breaker,
		l:                  l,
	}
}

// IncrReadCount 唯一走 Kafka 的写操作
func (s *KafkaInteractionService) IncrReadCount(ctx context.Context, biz string, bizId int64) error {
	if !s.breaker.Allow() {
		return s.InteractionService.IncrReadCount(ctx, biz, bizId)
	}
	evt := intrevt.InteractionEvent{Type: intrevt.TypeRead, Biz: biz, BizId: bizId}
	if err := s.producer.Produce(ctx, evt); err != nil {
		s.breaker.Fail()
		s.l.Error(ctx, "kafka 发送阅读事件失败，降级同步", logger.Error(err))
		return s.InteractionService.IncrReadCount(ctx, biz, bizId)
	}
	s.breaker.Success()
	return nil
}
