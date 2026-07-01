package ioc

import (
	"time"

	"github.com/IBM/sarama"
	"github.com/spf13/viper"

	"github.com/webook/internal/events"
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
	// 消费者侧配置已随 read 消费者迁出至 webook-worker（core 现仅生产）。
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

// InitEventProducer 懒连接 Kafka producer，不阻塞启动，详见 events.LazyProducer。
func InitEventProducer(kc KafkaConfig, cfg *sarama.Config, l logger.LoggerX) events.Producer {
	return events.NewLazyProducer(kc.Addrs, cfg, l, time.Second, 30*time.Second)
}
