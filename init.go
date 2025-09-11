package eotel

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"time"

	"github.com/getsentry/sentry-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.34.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

var globalTracer trace.Tracer
var globalMeter metric.Meter

func InitEOTEL(ctx context.Context, cfg Config) (func(context.Context) error, error) {
	globalCfg = cfg

	// --------- Build default exporter (ตาม config) ----------
	var exporters []Exporter
	if cfg.EnableLoki && cfg.LokiURL != "" {
		exporters = append(exporters, LokiExporter{})
	}
	if cfg.EnableSentry && cfg.SentryDSN != "" {
		exporters = append(exporters, SentryExporter{})
	}
	if len(exporters) > 0 {
		defaultExporter = NewMultiExporter(exporters...)
	} else {
		defaultExporter = nil
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(semconv.ServiceName(cfg.ServiceName)),
	)
	if err != nil {
		return nil, fmt.Errorf("resource.New: %w", err)
	}

	// ---------- OTLP connection options (TLS/Plain) ----------
	var traceOpts []otlptracegrpc.Option
	var metricOpts []otlpmetricgrpc.Option

	if cfg.OTLPUseTLS {
		tlsCfg := &tls.Config{} // สามารถเติม CA/ServerName ได้ในอนาคต
		creds := credentials.NewTLS(tlsCfg)
		traceOpts = append(traceOpts,
			otlptracegrpc.WithTLSCredentials(creds),
			otlptracegrpc.WithEndpoint(cfg.OtelCollector),
			otlptracegrpc.WithDialOption(grpc.WithBlock()),
		)
		metricOpts = append(metricOpts,
			otlpmetricgrpc.WithTLSCredentials(creds),
			otlpmetricgrpc.WithEndpoint(cfg.OtelCollector),
			otlpmetricgrpc.WithDialOption(grpc.WithBlock()),
		)
	} else {
		traceOpts = append(traceOpts,
			otlptracegrpc.WithInsecure(),
			otlptracegrpc.WithEndpoint(cfg.OtelCollector),
			otlptracegrpc.WithDialOption(grpc.WithBlock()),
		)
		metricOpts = append(metricOpts,
			otlpmetricgrpc.WithInsecure(),
			otlpmetricgrpc.WithEndpoint(cfg.OtelCollector),
			otlpmetricgrpc.WithDialOption(grpc.WithBlock()),
		)
	}

	// Init tracing
	if cfg.EnableTracing {
		tExp, err := otlptracegrpc.New(ctx, traceOpts...)
		if err != nil {
			return nil, fmt.Errorf("trace exporter: %w", err)
		}
		tp := sdktrace.NewTracerProvider(
			sdktrace.WithResource(res),
			sdktrace.WithSpanProcessor(sdktrace.NewBatchSpanProcessor(tExp)),
		)
		otel.SetTracerProvider(tp)
		globalTracer = tp.Tracer(cfg.ServiceName)
	} else {
		globalTracer = otel.GetTracerProvider().Tracer(cfg.ServiceName)
	}

	// Init metrics
	if cfg.EnableMetrics {
		mExp, err := otlpmetricgrpc.New(ctx, metricOpts...)
		if err != nil {
			return nil, fmt.Errorf("metric exporter: %w", err)
		}
		mp := sdkmetric.NewMeterProvider(
			sdkmetric.WithResource(res),
			sdkmetric.WithReader(sdkmetric.NewPeriodicReader(mExp)),
		)
		otel.SetMeterProvider(mp)
		globalMeter = mp.Meter(cfg.ServiceName)
	} else {
		globalMeter = otel.GetMeterProvider().Meter(cfg.ServiceName)
	}

	// Init sentry
	if cfg.EnableSentry {
		err := sentry.Init(sentry.ClientOptions{
			Dsn:              cfg.SentryDSN,
			EnableTracing:    cfg.EnableTracing,
			TracesSampleRate: 1.0,
			Environment:      "production",
		})
		if err != nil {
			log.Printf("init Sentry error: %v", err)
		}
	}

	// Graceful shutdown function
	return func(ctx context.Context) error {
		if cfg.EnableSentry {
			sentry.Flush(2 * time.Second)
		}
		return nil
	}, nil
}
