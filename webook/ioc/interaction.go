package ioc

import (
	"time"

	intrevt "gitee.com/train-cloud/geektime-basic-go/internal/events/interaction"
	"gitee.com/train-cloud/geektime-basic-go/internal/repository"
	"gitee.com/train-cloud/geektime-basic-go/internal/service"
	"gitee.com/train-cloud/geektime-basic-go/pkg/circuitbreaker"
	"gitee.com/train-cloud/geektime-basic-go/pkg/logger"
)

// InitInteractionService 组装互动 Service：Kafka 装饰器包装同步实现
func InitInteractionService(
	repo repository.InteractionRepository,
	producer intrevt.InteractionEventProducer,
	l logger.LoggerX,
) service.InteractionService {
	inner := service.NewInternalInteractionService(repo)
	breaker := circuitbreaker.NewBreaker(3, 30*time.Second)
	return service.NewKafkaInteractionService(inner, producer, breaker, l)
}
