package ioc

import (
	"time"

	"github.com/IBM/sarama"
	"github.com/spf13/viper"

	"github.com/webook/internal/events"
	"github.com/webook/pkg/logger"
)

// KafkaConfig 映射 yaml data.kafka 段（core 仅生产）。时间为 duration;超时缺省就地兜底。
type KafkaConfig struct {
	Addrs                []string      `mapstructure:"addrs"`
	DialTimeout          time.Duration `mapstructure:"dial_timeout"`
	ReadTimeout          time.Duration `mapstructure:"read_timeout"`
	WriteTimeout         time.Duration `mapstructure:"write_timeout"`
	ProducerTimeout      time.Duration `mapstructure:"producer_timeout"`
	ProducerRetryMax     int           `mapstructure:"producer_retry_max"` // 0=不重试(直接降级同步)
	MetadataRetryMax     int           `mapstructure:"metadata_retry_max"`
	MetadataRetryBackoff time.Duration `mapstructure:"metadata_retry_backoff"`
	MetadataTimeout      time.Duration `mapstructure:"metadata_timeout"`
}

func InitKafkaConfig() KafkaConfig {
	var cfg KafkaConfig
	if err := viper.UnmarshalKey("data.kafka", &cfg); err != nil {
		panic(err)
	}
	return cfg
}

// InitSaramaConfig 超时缺省就地兜底（各 10s / metadata 5s / backoff 250ms）；重试次数按 yaml（0=不重试）。
func InitSaramaConfig(kc KafkaConfig) *sarama.Config {
	cfg := sarama.NewConfig()
	cfg.Producer.Return.Successes = true
	cfg.Producer.Partitioner = sarama.NewHashPartitioner
	cfg.Net.DialTimeout = orDefaultDur(kc.DialTimeout, 10*time.Second)
	cfg.Net.ReadTimeout = orDefaultDur(kc.ReadTimeout, 10*time.Second)
	cfg.Net.WriteTimeout = orDefaultDur(kc.WriteTimeout, 10*time.Second)
	cfg.Producer.Timeout = orDefaultDur(kc.ProducerTimeout, 10*time.Second)
	cfg.Producer.Retry.Max = kc.ProducerRetryMax
	cfg.Metadata.Retry.Max = kc.MetadataRetryMax
	cfg.Metadata.Retry.Backoff = orDefaultDur(kc.MetadataRetryBackoff, 250*time.Millisecond)
	cfg.Metadata.Timeout = orDefaultDur(kc.MetadataTimeout, 5*time.Second)
	cfg.Consumer.Offsets.Initial = sarama.OffsetOldest
	return cfg
}

// orDefaultDur d<=0 时返回兜底默认。
func orDefaultDur(d, def time.Duration) time.Duration {
	if d <= 0 {
		return def
	}
	return d
}

// InitEventProducer 懒连接 Kafka producer，不阻塞启动，详见 events.LazyProducer。
func InitEventProducer(kc KafkaConfig, cfg *sarama.Config, l logger.LoggerX) events.Producer {
	return events.NewLazyProducer(kc.Addrs, cfg, l, time.Second, 30*time.Second)
}
