package ioc

import (
	"time"

	intrevt "github.com/webook/internal/events/interaction"
	"github.com/webook/internal/repository"
	"github.com/webook/internal/service"
	"github.com/webook/pkg/circuitbreaker"
	"github.com/webook/pkg/logger"
)

// InitInteractionService 组装互动 Service：RankingAware → Kafka → Internal
func InitInteractionService(
	repo repository.InteractionRepository,
	rankRepo repository.RankingRepository,
	producer intrevt.InteractionEventProducer,
	l logger.LoggerX,
) service.InteractionService {
	inner := service.NewInternalInteractionService(repo)
	breaker := circuitbreaker.NewBreaker(3, 30*time.Second)
	withKafka := service.NewKafkaInteractionService(inner, producer, breaker, l)
	return service.NewArticleRankingAwareInteractionService(withKafka, rankRepo, l)
}
