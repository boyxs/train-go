package ioc

import (
	"context"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	interactionv1 "github.com/boyxs/train-go/webook/api/gen/interaction/v1"
	intrevt "github.com/boyxs/train-go/webook/internal/events/interaction"
	"github.com/boyxs/train-go/webook/internal/repository"
	"github.com/boyxs/train-go/webook/internal/service"
	"github.com/boyxs/train-go/webook/pkg/circuitbreaker"
	"github.com/boyxs/train-go/webook/pkg/logger"
	"github.com/boyxs/train-go/webook/pkg/pool"
)

// ranking boost 协程池规模：固定 worker + 有界队列。队列满即丢弃（cron 全量重算兜底）。
const (
	rankingBoostWorkers   = 16
	rankingBoostQueueSize = 1024
)

// InitRankingBoostPool ranking 实时 boost 用的协程池。默认 Discard（队列满即丢，cron 兜底）；
// cleanup=Shutdown，主停机时优雅排空在途/已入队任务。指标在 InitInteractionService 装配时统一注册。
func InitRankingBoostPool(l logger.LoggerX) (*pool.Pool, func()) {
	p := pool.New(rankingBoostWorkers, rankingBoostQueueSize,
		pool.WithPanicHandler(func(r any) {
			l.Error(context.Background(), "ranking boost 任务 panic", logger.String("panic", fmt.Sprintf("%v", r)))
		}))
	return p, p.Shutdown
}

// InitInteractionService 组装互动 Service：RankingAware（boost 协程池）→ Kafka 降级 → gRPC 适配器。
// 互动数据已拆为独立 webook-interaction 服务，core 经 gRPC client 调用；read 计数的 Kafka 异步+熔断降级
// 与 ranking 实时 boost（协程池异步、队列满丢弃、cron 兜底）两个横切由 core 装饰器追加。
func InitInteractionService(
	client interactionv1.InteractionServiceClient,
	rankRepo repository.RankingRepository,
	boostPool *pool.Pool,
	producer intrevt.InteractionEventProducer,
	l logger.LoggerX,
) service.InteractionService {
	inner := service.NewGRPCInteractionService(client)
	breaker := circuitbreaker.NewBreaker(3, 30*time.Second)
	withKafka := service.NewKafkaInteractionService(inner, producer, breaker, l)
	return service.NewArticleRankingAwareInteractionService(withKafka, rankRepo, boostPool, registerBoostMetrics(boostPool), l)
}

// registerBoostMetrics 一处注册全部 ranking boost 指标到默认 registry（/metrics 暴露），返回供装饰器 Inc 的 dropped 计数器。
// 进程内只调一次（InitInteractionService 是 wire 单例），不会重复注册。
//   - webook_ranking_boost_pool_{queued,in_flight,capacity} 池运行态（GaugeFunc 拉取，池本身不依赖 prometheus）
//   - webook_ranking_boost_dropped_total                    队列满丢弃次数（装饰器 Inc）
func registerBoostMetrics(p *pool.Pool) prometheus.Counter {
	capacity := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "webook", Subsystem: "ranking", Name: "boost_pool_capacity",
		Help: "ranking boost 协程池队列容量",
	})
	capacity.Set(float64(rankingBoostQueueSize))
	dropped := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "webook", Subsystem: "ranking", Name: "boost_dropped_total",
		Help: "ranking 实时 boost 因协程池队列满被丢弃的次数",
	})
	prometheus.MustRegister(
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Namespace: "webook", Subsystem: "ranking", Name: "boost_pool_queued",
			Help: "ranking boost 协程池当前排队任务数",
		}, func() float64 { return float64(p.Queued()) }),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Namespace: "webook", Subsystem: "ranking", Name: "boost_pool_in_flight",
			Help: "ranking boost 协程池当前在途（in-flight）任务数",
		}, func() float64 { return float64(p.InFlight()) }),
		capacity,
		dropped,
	)
	return dropped
}
