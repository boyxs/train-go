package ioc

import (
	"time"

	"github.com/IBM/sarama"
	"github.com/spf13/viper"

	"github.com/webook/worker/consumer"
)

// KafkaConfig 映射 yaml kafka 段（worker 只消费，不生产）。
type KafkaConfig struct {
	Addrs               []string `mapstructure:"addrs"`
	DialTimeout         int      `mapstructure:"dialTimeout"`
	ReadTimeout         int      `mapstructure:"readTimeout"`
	WriteTimeout        int      `mapstructure:"writeTimeout"`
	ConsumerGroup       string   `mapstructure:"consumerGroup"`
	ConsumerBackoffInit int      `mapstructure:"consumerBackoffInitial"`
	ConsumerBackoffMax  int      `mapstructure:"consumerBackoffMax"`
}

func InitKafkaConfig() KafkaConfig {
	var kc KafkaConfig
	if err := viper.UnmarshalKey("kafka", &kc); err != nil {
		panic(err)
	}
	return kc
}

func InitSaramaConfig(kc KafkaConfig) *sarama.Config {
	cfg := sarama.NewConfig()
	if kc.DialTimeout > 0 {
		cfg.Net.DialTimeout = time.Duration(kc.DialTimeout) * time.Second
	}
	if kc.ReadTimeout > 0 {
		cfg.Net.ReadTimeout = time.Duration(kc.ReadTimeout) * time.Second
	}
	if kc.WriteTimeout > 0 {
		cfg.Net.WriteTimeout = time.Duration(kc.WriteTimeout) * time.Second
	}
	cfg.Consumer.Offsets.Initial = sarama.OffsetOldest // worker 晚启动也能补消费
	return cfg
}

func InitConsumerConfig(kc KafkaConfig) consumer.ConsumerConfig {
	return consumer.ConsumerConfig{
		Addrs:          kc.Addrs,
		GroupID:        kc.ConsumerGroup,
		BackoffInitial: time.Duration(kc.ConsumerBackoffInit) * time.Second,
		BackoffMax:     time.Duration(kc.ConsumerBackoffMax) * time.Second,
	}
}
