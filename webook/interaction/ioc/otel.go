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

	"github.com/boyxs/train-go/webook/shared/confkey"
)

// InitOTel 初始化 OpenTelemetry（OTLP/gRPC），service.name 取 otel.service_name。
func InitOTel() (trace.TracerProvider, func(), error) {
	type Config struct {
		Endpoint       string  `mapstructure:"endpoint"`
		ServiceName    string  `mapstructure:"service_name"`
		ServiceVersion string  `mapstructure:"service_version"`
		Env            string  `mapstructure:"env"`
		SampleRatio    float64 `mapstructure:"sample_ratio"`
	}
	cfg := Config{
		Endpoint:       "localhost:4317",
		ServiceName:    "webook-interaction",
		ServiceVersion: "0.1.0",
		Env:            "dev",
		SampleRatio:    1.0,
	}
	if err := viper.UnmarshalKey(confkey.OTel, &cfg); err != nil {
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
			log.Printf("[OTel] TracerProvider shutdown failed: %v", err)
		}
	}
	return tp, cleanup, nil
}
