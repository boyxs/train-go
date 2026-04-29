package ioc

import (
	"log"
	"time"

	"github.com/IBM/sarama"
	"github.com/spf13/viper"

	"github.com/webook/internal/events"
	intrevt "github.com/webook/internal/events/interaction"
	"github.com/webook/pkg/logger"
)

// KafkaConfig 映射 yaml 里 kafka 段
type KafkaConfig struct {
	Addrs                []string `mapstructure:"addrs"`
	DialTimeout          int      `mapstructure:"dialTimeout"`     // 秒
	ReadTimeout          int      `mapstructure:"readTimeout"`     // 秒
	WriteTimeout         int      `mapstructure:"writeTimeout"`    // 秒
	ProducerTimeout      int      `mapstructure:"producerTimeout"` // 秒
	ProducerRetryMax     int      `mapstructure:"producerRetryMax"`
	MetadataRetryMax     int      `mapstructure:"metadataRetryMax"`
	MetadataRetryBackoff int      `mapstructure:"metadataRetryBackoff"` // 毫秒
	MetadataTimeout      int      `mapstructure:"metadataTimeout"`      // 秒
	ConsumerGroup        string   `mapstructure:"consumerGroup"`
	ConsumerBackoffInit  int      `mapstructure:"consumerBackoffInitial"` // 秒
	ConsumerBackoffMax   int      `mapstructure:"consumerBackoffMax"`     // 秒
}

// InitKafkaConfig 读取 yaml 配置
func InitKafkaConfig() KafkaConfig {
	var cfg KafkaConfig
	if err := viper.UnmarshalKey("kafka", &cfg); err != nil {
		panic(err)
	}
	return cfg
}

// InitSaramaConfig sarama 底层配置
func InitSaramaConfig(kc KafkaConfig) *sarama.Config {
	cfg := sarama.NewConfig()
	cfg.Producer.Return.Successes = true
	cfg.Producer.Partitioner = sarama.NewHashPartitioner
	cfg.Net.DialTimeout = time.Duration(kc.DialTimeout) * time.Second
	cfg.Net.ReadTimeout = time.Duration(kc.ReadTimeout) * time.Second
	cfg.Net.WriteTimeout = time.Duration(kc.WriteTimeout) * time.Second
	cfg.Producer.Timeout = time.Duration(kc.ProducerTimeout) * time.Second
	cfg.Producer.Retry.Max = kc.ProducerRetryMax
	cfg.Metadata.Retry.Max = kc.MetadataRetryMax
	cfg.Metadata.Retry.Backoff = time.Duration(kc.MetadataRetryBackoff) * time.Millisecond
	cfg.Metadata.Timeout = time.Duration(kc.MetadataTimeout) * time.Second
	cfg.Consumer.Offsets.Initial = sarama.OffsetOldest
	return cfg
}

// InitSaramaSyncProducer Kafka 连接失败不 panic，返回 nil
// OTel：trace 注入/提取在 events.SaramaSyncProducer.ProduceEvent 和 pkg/saramax.BatchConsumer 里做
// （因为 sarama.SendMessage 签名无 ctx，只能在高层接上 ctx 的地方做）
//
// 重试：应对 webook 启动早于 Kafka JVM ready 的竞态（指数退避 6 次，共约 63 秒窗口）
func InitSaramaSyncProducer(kc KafkaConfig, cfg *sarama.Config) sarama.SyncProducer {
	producer, ok := retryConnect("Kafka producer", 6, time.Second, func() (sarama.SyncProducer, error) {
		return sarama.NewSyncProducer(kc.Addrs, cfg)
	})
	if !ok {
		log.Printf("[Kafka] producer 连接重试耗尽，写操作降级为 NoopProducer")
		return nil
	}
	return producer
}

// InitSaramaClient Kafka 连接失败不 panic，返回 nil；同样带重试防启动竞态
func InitSaramaClient(kc KafkaConfig, cfg *sarama.Config) sarama.Client {
	client, ok := retryConnect("Kafka client", 6, time.Second, func() (sarama.Client, error) {
		return sarama.NewClient(kc.Addrs, cfg)
	})
	if !ok {
		log.Printf("[Kafka] client 连接重试耗尽，consumer 不启动")
		return nil
	}
	return client
}

// retryConnect 通用连接重试：指数退避直到成功或次数耗尽。
// backoff 每次翻倍，上限 30 秒。失败到达 attempts 次后返回零值 + false。
// label 完整用作日志前缀，方便未来给 ES / Mongo 等其它中间件复用（不再写死 [Kafka]）。
func retryConnect[T any](label string, attempts int, initialBackoff time.Duration, fn func() (T, error)) (T, bool) {
	var zero T
	backoff := initialBackoff
	for i := 1; i <= attempts; i++ {
		v, err := fn()
		if err == nil {
			if i > 1 {
				log.Printf("[%s] 第 %d 次重试连接成功", label, i)
			}
			return v, true
		}
		log.Printf("[%s] 第 %d/%d 次连接失败: %v", label, i, attempts, err)
		if i < attempts {
			time.Sleep(backoff)
			if backoff < 30*time.Second {
				backoff *= 2
			}
		}
	}
	return zero, false
}

// InitInteractionConsumerConfig 互动 Consumer 的配置
func InitInteractionConsumerConfig(kc KafkaConfig) intrevt.ConsumerConfig {
	return intrevt.ConsumerConfig{
		GroupID:        kc.ConsumerGroup,
		BackoffInitial: time.Duration(kc.ConsumerBackoffInit) * time.Second,
		BackoffMax:     time.Duration(kc.ConsumerBackoffMax) * time.Second,
	}
}

// InitEventProducer producer 为 nil 时返回 noop
func InitEventProducer(producer sarama.SyncProducer, l logger.LoggerX) events.Producer {
	if producer == nil {
		return &events.NoopProducer{}
	}
	return events.NewSaramaSyncProducer(producer, l)
}
