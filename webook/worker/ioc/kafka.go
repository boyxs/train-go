package ioc

import (
	"time"

	"github.com/IBM/sarama"
	"github.com/spf13/viper"

	"github.com/boyxs/train-go/webook/pkg/saramax"
	"github.com/boyxs/train-go/webook/shared/confkey"
)

// KafkaConfig 映射 yaml data.kafka 段（worker 只消费，不生产）。时间为 duration;缺省就地兜底。
type KafkaConfig struct {
	Addrs               []string      `mapstructure:"addrs"`
	DialTimeout         time.Duration `mapstructure:"dial_timeout"`
	ReadTimeout         time.Duration `mapstructure:"read_timeout"`
	WriteTimeout        time.Duration `mapstructure:"write_timeout"`
	ConsumerGroup       string        `mapstructure:"consumer_group"`
	ConsumerBackoffInit time.Duration `mapstructure:"consumer_backoff_initial"`
	ConsumerBackoffMax  time.Duration `mapstructure:"consumer_backoff_max"`
}

func InitKafkaConfig() KafkaConfig {
	var kc KafkaConfig
	if err := viper.UnmarshalKey(confkey.DataKafka, &kc); err != nil {
		panic(err)
	}
	return kc
}

// InitSaramaConfig 就地兜底默认（yaml 缺省时）：dial/read/write 各 10s。
func InitSaramaConfig(kc KafkaConfig) *sarama.Config {
	cfg := sarama.NewConfig()
	cfg.Net.DialTimeout = orDefaultDur(kc.DialTimeout, 10*time.Second)
	cfg.Net.ReadTimeout = orDefaultDur(kc.ReadTimeout, 10*time.Second)
	cfg.Net.WriteTimeout = orDefaultDur(kc.WriteTimeout, 10*time.Second)
	cfg.Consumer.Offsets.Initial = sarama.OffsetOldest // worker 晚启动也能补消费
	return cfg
}

// InitGroupConfig 就地兜底：退避 initial 5s / max 60s。
func InitGroupConfig(kc KafkaConfig) saramax.GroupConfig {
	return saramax.GroupConfig{
		Addrs:          kc.Addrs,
		GroupId:        kc.ConsumerGroup,
		BackoffInitial: orDefaultDur(kc.ConsumerBackoffInit, 5*time.Second),
		BackoffMax:     orDefaultDur(kc.ConsumerBackoffMax, 60*time.Second),
	}
}

// orDefaultDur d<=0 时返回兜底默认。
func orDefaultDur(d, def time.Duration) time.Duration {
	if d <= 0 {
		return def
	}
	return d
}
