package ioc

import (
	"log"
	"time"

	"gitee.com/train-cloud/geektime-basic-go/internal/events"
	intrevt "gitee.com/train-cloud/geektime-basic-go/internal/events/interaction"
	"gitee.com/train-cloud/geektime-basic-go/pkg/logger"
	"github.com/IBM/sarama"
	"github.com/spf13/viper"
)

// KafkaConfig 映射 yaml 里 kafka 段
type KafkaConfig struct {
	Addrs                []string `mapstructure:"addrs"`
	DialTimeout          int      `mapstructure:"dialTimeout"`          // 秒
	ReadTimeout          int      `mapstructure:"readTimeout"`          // 秒
	WriteTimeout         int      `mapstructure:"writeTimeout"`         // 秒
	ProducerTimeout      int      `mapstructure:"producerTimeout"`      // 秒
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
func InitSaramaSyncProducer(kc KafkaConfig, cfg *sarama.Config) sarama.SyncProducer {
	producer, err := sarama.NewSyncProducer(kc.Addrs, cfg)
	if err != nil {
		log.Printf("[Kafka] producer 连接失败，写操作将降级同步: %v", err)
		return nil
	}
	return producer
}

// InitSaramaClient Kafka 连接失败不 panic，返回 nil
func InitSaramaClient(kc KafkaConfig, cfg *sarama.Config) sarama.Client {
	client, err := sarama.NewClient(kc.Addrs, cfg)
	if err != nil {
		log.Printf("[Kafka] client 连接失败，consumer 不启动: %v", err)
		return nil
	}
	return client
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
