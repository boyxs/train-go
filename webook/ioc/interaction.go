package ioc

import (
	"time"

	intrevt "github.com/webook/internal/events/interaction"
	"github.com/webook/internal/repository"
	"github.com/webook/internal/service"
	"github.com/webook/pkg/circuitbreaker"
	"github.com/webook/pkg/logger"
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
