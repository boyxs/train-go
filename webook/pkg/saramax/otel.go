package saramax

import (
	"context"

	"github.com/IBM/sarama"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// MessagingSystem = "kafka" per OTel semconv
const messagingSystem = "kafka"

// ── HeadersCarrier：把 sarama message headers 适配成 TextMapCarrier ─────────

// ProducerHeadersCarrier 用于 Producer 消息的 Headers（*sarama.ProducerMessage.Headers 是 []sarama.RecordHeader 值类型）
type ProducerHeadersCarrier struct {
	msg *sarama.ProducerMessage
}

func NewProducerHeadersCarrier(msg *sarama.ProducerMessage) ProducerHeadersCarrier {
	return ProducerHeadersCarrier{msg: msg}
}

func (c ProducerHeadersCarrier) Get(key string) string {
	for _, h := range c.msg.Headers {
		if string(h.Key) == key {
			return string(h.Value)
		}
	}
	return ""
}

func (c ProducerHeadersCarrier) Set(key, value string) {
	// 先删重名，再追加（避免重复键）
	out := c.msg.Headers[:0]
	for _, h := range c.msg.Headers {
		if string(h.Key) != key {
			out = append(out, h)
		}
	}
	out = append(out, sarama.RecordHeader{
		Key:   []byte(key),
		Value: []byte(value),
	})
	c.msg.Headers = out
}

func (c ProducerHeadersCarrier) Keys() []string {
	keys := make([]string, 0, len(c.msg.Headers))
	for _, h := range c.msg.Headers {
		keys = append(keys, string(h.Key))
	}
	return keys
}

// ConsumerHeadersCarrier 用于 Consumer 消息的 Headers（*sarama.ConsumerMessage.Headers 是 []*sarama.RecordHeader 指针）
type ConsumerHeadersCarrier struct {
	msg *sarama.ConsumerMessage
}

func NewConsumerHeadersCarrier(msg *sarama.ConsumerMessage) ConsumerHeadersCarrier {
	return ConsumerHeadersCarrier{msg: msg}
}

func (c ConsumerHeadersCarrier) Get(key string) string {
	for _, h := range c.msg.Headers {
		if h != nil && string(h.Key) == key {
			return string(h.Value)
		}
	}
	return ""
}

func (c ConsumerHeadersCarrier) Set(key, value string) {
	out := c.msg.Headers[:0]
	for _, h := range c.msg.Headers {
		if h != nil && string(h.Key) != key {
			out = append(out, h)
		}
	}
	out = append(out, &sarama.RecordHeader{
		Key:   []byte(key),
		Value: []byte(value),
	})
	c.msg.Headers = out
}

func (c ConsumerHeadersCarrier) Keys() []string {
	keys := make([]string, 0, len(c.msg.Headers))
	for _, h := range c.msg.Headers {
		if h != nil {
			keys = append(keys, string(h.Key))
		}
	}
	return keys
}

var _ propagation.TextMapCarrier = ProducerHeadersCarrier{}
var _ propagation.TextMapCarrier = ConsumerHeadersCarrier{}

// ── Producer 侧 ────────────────────────────────────────────────────────

// StartProducerSpan 发送前创建 span，把 trace context 注入到 message headers
// 返回 span 让调用方 End（失败时 RecordError）
func StartProducerSpan(ctx context.Context, msg *sarama.ProducerMessage) (context.Context, trace.Span) {
	tracer := otel.Tracer("webook/pkg/saramax")
	ctx, span := tracer.Start(ctx,
		"kafka send "+msg.Topic,
		trace.WithSpanKind(trace.SpanKindProducer),
		trace.WithAttributes(
			attribute.String("messaging.system", messagingSystem),
			attribute.String("messaging.destination.name", msg.Topic),
			attribute.String("messaging.operation", "publish"),
		),
	)
	// 把 ctx 里的 trace context 注入到 msg.Headers
	otel.GetTextMapPropagator().Inject(ctx, NewProducerHeadersCarrier(msg))
	return ctx, span
}

// ── Consumer 侧 ────────────────────────────────────────────────────────

// StartConsumerSpan 从 message headers 提取 trace context，创建 consumer span
// 调用方在 handler 执行前调用，用返回的 ctx 透传下去
func StartConsumerSpan(ctx context.Context, msg *sarama.ConsumerMessage) (context.Context, trace.Span) {
	// 从 headers 提取 producer 写入的 trace context
	parentCtx := otel.GetTextMapPropagator().Extract(ctx, NewConsumerHeadersCarrier(msg))
	tracer := otel.Tracer("webook/pkg/saramax")
	return tracer.Start(parentCtx,
		"kafka receive "+msg.Topic,
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(
			attribute.String("messaging.system", messagingSystem),
			attribute.String("messaging.destination.name", msg.Topic),
			attribute.String("messaging.operation", "receive"),
			attribute.Int64("messaging.kafka.message.offset", msg.Offset),
			attribute.Int("messaging.kafka.destination.partition", int(msg.Partition)),
		),
	)
}

// ExtractTraceContext 仅从消息 headers 提取上游（producer）trace context，不新建 span。
// 用于 consumer span 建立之前/之外的日志点（如反序列化失败）也能带上 trace_id，追回是哪个 producer 发的。
func ExtractTraceContext(msg *sarama.ConsumerMessage) context.Context {
	return otel.GetTextMapPropagator().Extract(context.Background(), NewConsumerHeadersCarrier(msg))
}

// RecordSpanError 统一设置 span 错误状态
func RecordSpanError(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
}

// startBatchConsumerSpan 为批量消费创建一个 Consumer span
// 第一条消息的 headers 作为 parent trace context，其它消息作为 span links
// 这样不需要改动 BatchHandler 签名，调用方依然零侵入
func startBatchConsumerSpan(ctx context.Context, msgs []*sarama.ConsumerMessage) (context.Context, trace.Span) {
	if len(msgs) == 0 {
		return otel.Tracer("webook/pkg/saramax").Start(ctx, "kafka receive batch(empty)")
	}
	first := msgs[0]
	parentCtx := otel.GetTextMapPropagator().Extract(ctx, NewConsumerHeadersCarrier(first))

	// 剩余消息作为 links，让 trace UI 能展示多源关联
	links := make([]trace.Link, 0, len(msgs)-1)
	for i := 1; i < len(msgs); i++ {
		linkCtx := otel.GetTextMapPropagator().Extract(ctx, NewConsumerHeadersCarrier(msgs[i]))
		sc := trace.SpanContextFromContext(linkCtx)
		if sc.IsValid() {
			links = append(links, trace.Link{SpanContext: sc})
		}
	}

	return otel.Tracer("webook/pkg/saramax").Start(parentCtx,
		"kafka consume "+first.Topic,
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(
			attribute.String("messaging.system", messagingSystem),
			attribute.String("messaging.destination.name", first.Topic),
			attribute.String("messaging.operation", "process"),
			attribute.Int("messaging.batch.message_count", len(msgs)),
		),
		trace.WithLinks(links...),
	)
}
