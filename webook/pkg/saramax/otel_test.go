package saramax

import (
	"context"
	"testing"

	"github.com/IBM/sarama"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func init() {
	// 测试全局注册 W3C propagator（生产在 ioc.InitOTel 里做）
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
}

// ── ProducerHeadersCarrier ───────────────────────────────────────────

func TestProducerHeadersCarrier_SetGet(t *testing.T) {
	msg := &sarama.ProducerMessage{}
	c := NewProducerHeadersCarrier(msg)

	c.Set("traceparent", "abc123")
	c.Set("tracestate", "vendor=x")

	assert.Equal(t, "abc123", c.Get("traceparent"))
	assert.Equal(t, "vendor=x", c.Get("tracestate"))
	assert.Equal(t, "", c.Get("missing"))
	assert.ElementsMatch(t, []string{"traceparent", "tracestate"}, c.Keys())
}

func TestProducerHeadersCarrier_SetOverrides(t *testing.T) {
	msg := &sarama.ProducerMessage{
		Headers: []sarama.RecordHeader{
			{Key: []byte("traceparent"), Value: []byte("old")},
			{Key: []byte("other"), Value: []byte("keep")},
		},
	}
	c := NewProducerHeadersCarrier(msg)
	c.Set("traceparent", "new")

	assert.Equal(t, "new", c.Get("traceparent"))
	assert.Equal(t, "keep", c.Get("other"))
	assert.Len(t, msg.Headers, 2)
}

// ── ConsumerHeadersCarrier ───────────────────────────────────────────

func TestConsumerHeadersCarrier_GetWithNilEntry(t *testing.T) {
	msg := &sarama.ConsumerMessage{
		Headers: []*sarama.RecordHeader{
			{Key: []byte("traceparent"), Value: []byte("xyz")},
			nil, // 真实 sarama 偶发场景，不能 panic
			{Key: []byte("other"), Value: []byte("v")},
		},
	}
	c := NewConsumerHeadersCarrier(msg)

	assert.Equal(t, "xyz", c.Get("traceparent"))
	assert.Equal(t, "v", c.Get("other"))
	assert.ElementsMatch(t, []string{"traceparent", "other"}, c.Keys())
}

// ── 端到端：Inject → Extract 链路连续 ───────────────────────────────────

func TestInjectExtract_PreservesTraceContext(t *testing.T) {
	// 用内存 exporter 捕获 span，不走网络
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	defer tp.Shutdown(context.Background())

	prevTP := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	defer otel.SetTracerProvider(prevTP)

	// Producer 侧：创建 span + 注入 header
	producerTracer := tp.Tracer("producer-test")
	rootCtx, rootSpan := producerTracer.Start(context.Background(), "test-root")
	defer rootSpan.End()

	producerMsg := &sarama.ProducerMessage{Topic: "test-topic"}
	_, pSpan := StartProducerSpan(rootCtx, producerMsg)
	pSpan.End()

	require.NotEmpty(t, producerMsg.Headers, "Inject 必须写入 traceparent header")

	// 模拟消息过 broker 后到消费端：header 透传
	consumerMsg := &sarama.ConsumerMessage{
		Topic:     "test-topic",
		Partition: 0,
		Offset:    100,
	}
	for _, h := range producerMsg.Headers {
		consumerMsg.Headers = append(consumerMsg.Headers, &sarama.RecordHeader{
			Key: h.Key, Value: h.Value,
		})
	}

	// Consumer 侧：提取 + 创建 span
	_, cSpan := StartConsumerSpan(context.Background(), consumerMsg)
	cSpan.End()

	// 验证 consumer span 的 TraceID 与 producer span 相同
	spans := recorder.Ended()
	var producerSC, consumerSC trace.SpanContext
	for _, s := range spans {
		switch s.Name() {
		case "kafka send test-topic":
			producerSC = s.SpanContext()
		case "kafka receive test-topic":
			consumerSC = s.SpanContext()
		}
	}
	require.True(t, producerSC.IsValid(), "producer span 未捕获")
	require.True(t, consumerSC.IsValid(), "consumer span 未捕获")
	assert.Equal(t, producerSC.TraceID(), consumerSC.TraceID(),
		"producer 和 consumer 必须在同一 trace")
}
