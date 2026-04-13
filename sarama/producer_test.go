package kafka

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/IBM/sarama"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var addrs = []string{"localhost:9094"}

func TestSyncProducer(t *testing.T) {
	cfg := sarama.NewConfig()
	// SyncProducer 必须开启，否则 panic
	cfg.Producer.Return.Successes = true
	// 分区策略：先配置再创建 Producer
	cfg.Producer.Partitioner = sarama.NewRoundRobinPartitioner
	//cfg.Producer.Partitioner = sarama.NewRandomPartitioner
	//cfg.Producer.Partitioner = sarama.NewHashPartitioner
	//cfg.Producer.Partitioner = sarama.NewManualPartitioner
	//cfg.Producer.Partitioner = sarama.NewConsistentCRCHashPartitioner

	producer, err := sarama.NewSyncProducer(addrs, cfg)
	require.NoError(t, err)
	defer producer.Close()

	for i := 0; i < 100; i++ {
		partition, offset, err := producer.SendMessage(&sarama.ProducerMessage{
			Topic: "test_topic",
			Key:   sarama.StringEncoder(fmt.Sprintf("key_%d", i)),
			Value: sarama.StringEncoder("这是一条测试消息"),
			Headers: []sarama.RecordHeader{
				{
					Key:   []byte("key"),
					Value: []byte("value"),
				},
			},
			Metadata: "这是元数据信息",
		})
		assert.NoError(t, err)
		t.Logf("sent #%d → partition=%d offset=%d", i, partition, offset)
	}
}

func TestAsyncProducer(t *testing.T) {
	cfg := sarama.NewConfig()
	cfg.Producer.Return.Successes = true
	cfg.Producer.Return.Errors = true
	cfg.Producer.Partitioner = sarama.NewRoundRobinPartitioner

	producer, err := sarama.NewAsyncProducer(addrs, cfg)
	require.NoError(t, err)

	// 后台收集结果：必须消费 Successes 和 Errors，否则 producer 会阻塞
	var wg sync.WaitGroup
	var successCount, errCount int64
	wg.Add(2)
	go func() {
		defer wg.Done()
		for range producer.Successes() {
			atomic.AddInt64(&successCount, 1)
		}
	}()
	go func() {
		defer wg.Done()
		for e := range producer.Errors() {
			atomic.AddInt64(&errCount, 1)
			t.Logf("失败: %v", e.Err)
		}
	}()

	// 批量发送
	for i := 0; i < 100; i++ {
		producer.Input() <- &sarama.ProducerMessage{
			Topic: "test_topic",
			Key:   sarama.StringEncoder(fmt.Sprintf("key_%d", i)),
			Value: sarama.StringEncoder(fmt.Sprintf("异步消息 #%d", i)),
			Headers: []sarama.RecordHeader{
				{
					Key:   []byte("key"),
					Value: []byte("value"),
				},
			},
			Metadata: "这是元数据信息",
		}
	}

	// AsyncClose 不阻塞，让后台 goroutine 消费完 Successes/Errors 后自然退出
	producer.AsyncClose()
	wg.Wait()

	t.Logf("总计: 成功=%d 失败=%d", successCount, errCount)
	assert.Equal(t, int64(100), successCount)
	assert.Equal(t, int64(0), errCount)
}
