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
	"go.opentelemetry.io/otel/trace/noop"
)

// InitOTel 初始化 OpenTelemetry：OTLP/gRPC → otel-collector。
//
// yaml 开关：
//
//	otel.disabled: true  返回 noop tracer（本地无 collector 时用，避免 export 重试 log 刷屏）
func InitOTel() (trace.TracerProvider, func(), error) {
	if viper.GetBool("otel.disabled") {
		log.Printf("[migrator] OTel disabled via yaml; using noop tracer")
		return noop.NewTracerProvider(), func() {}, nil
	}
	type Config struct {
		Endpoint       string  `yaml:"endpoint"`
		ServiceName    string  `yaml:"serviceName"`
		ServiceVersion string  `yaml:"serviceVersion"`
		Env            string  `yaml:"env"`
		SampleRatio    float64 `yaml:"sampleRatio"`
	}
	cfg := Config{
		Endpoint:       "localhost:4317",
		ServiceName:    "webook-migrator",
		ServiceVersion: "0.1.0",
		Env:            "dev",
		SampleRatio:    1.0,
	}
	if err := viper.UnmarshalKey("otel", &cfg); err != nil {
		return nil, nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(cfg.Endpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, nil, err
	}

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
		sdktrace.WithBatcher(exporter,
			sdktrace.WithBatchTimeout(5*time.Second),
			sdktrace.WithMaxExportBatchSize(512),
			sdktrace.WithMaxQueueSize(2048),
		),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(
			sdktrace.TraceIDRatioBased(cfg.SampleRatio),
		)),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	cleanup := func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := tp.Shutdown(shutdownCtx); err != nil {
			log.Printf("[migrator] OTel TracerProvider shutdown failed: %v", err)
		}
	}
	return tp, cleanup, nil
}
