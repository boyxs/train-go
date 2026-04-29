package ioc

import (
	"context"
	"log"
	"time"

	"github.com/spf13/viper"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
	"go.opentelemetry.io/otel/trace"
)

// InitOTel 初始化 OpenTelemetry：OTLP/gRPC → otel-collector → Zipkin
//
// 标准协议（CNCF OTLP），切换后端只改 Collector 出口配置，业务代码零改动。
// 容灾在 collector 层：sending_queue + retry_on_failure + file_storage 持久化（详见 deploy/otel-collector/config.yaml）。
//
// 返回 cleanup 由 wire 收集、main 退出时调用，确保 BatchSpanProcessor flush。
func InitOTel() (trace.TracerProvider, func(), error) {
	type Config struct {
		Endpoint       string  `yaml:"endpoint"`
		ServiceName    string  `yaml:"serviceName"`
		ServiceVersion string  `yaml:"serviceVersion"`
		Env            string  `yaml:"env"`
		SampleRatio    float64 `yaml:"sampleRatio"`
	}
	cfg := Config{
		Endpoint:       "localhost:4317",
		ServiceName:    "webook-core",
		ServiceVersion: "0.1.0",
		Env:            "dev",
		SampleRatio:    1.0,
	}
	if err := viper.UnmarshalKey("otel", &cfg); err != nil {
		return nil, nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// OTLP gRPC exporter：lazy 连接，Collector 暂时下线不影响应用启动
	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(cfg.Endpoint),
		otlptracegrpc.WithInsecure(), // 内网无 TLS
	)
	if err != nil {
		return nil, nil, err
	}

	// Resource：service.name 是后端 UI 分组的关键字段
	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion(cfg.ServiceVersion),
			attribute.String("deployment.environment", cfg.Env),
		),
	)
	if err != nil {
		return nil, nil, err
	}

	tp := sdktrace.NewTracerProvider(
		// BatchSpanProcessor 生产标配，攒批发送降低 IO
		sdktrace.WithBatcher(exporter,
			sdktrace.WithBatchTimeout(5*time.Second),
			sdktrace.WithMaxExportBatchSize(512),
			sdktrace.WithMaxQueueSize(2048),
		),
		sdktrace.WithResource(res),
		// ParentBased 跟随上游决策，无父 span 时按 ratio 采样
		sdktrace.WithSampler(sdktrace.ParentBased(
			sdktrace.TraceIDRatioBased(cfg.SampleRatio),
		)),
	)

	// 全局注册：业务代码 otel.Tracer(scope) 拿到这个 Provider
	otel.SetTracerProvider(tp)
	// 跨进程传播：W3C TraceContext + Baggage 是 OTel spec 默认
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	cleanup := func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := tp.Shutdown(shutdownCtx); err != nil {
			log.Printf("[OTel] TracerProvider shutdown failed: %v", err)
		}
	}
	return tp, cleanup, nil
}
