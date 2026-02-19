// Package otel provides OpenTelemetry tracing and metrics setup for CodeForge.
package otel

import (
	"context"
	"errors"
	"log/slog"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// ShutdownFunc is called to flush and shut down the trace and meter providers.
type ShutdownFunc func(ctx context.Context) error

// OTELConfig holds configuration for OpenTelemetry initialization.
type OTELConfig struct {
	Enabled     bool
	Endpoint    string
	ServiceName string
	Insecure    bool
	SampleRate  float64
}

// InitTracer initializes OpenTelemetry TracerProvider and MeterProvider.
// When cfg.Enabled is false, global providers remain as no-ops and a no-op
// shutdown function is returned.
func InitTracer(cfg OTELConfig) (ShutdownFunc, error) {
	if !cfg.Enabled {
		slog.Info("otel: disabled, using no-op providers")
		return func(_ context.Context) error { return nil }, nil
	}

	ctx := context.Background()

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(cfg.ServiceName),
		),
	)
	if err != nil {
		return nil, err
	}

	// Build gRPC dial options.
	dialOpts := []grpc.DialOption{}
	if cfg.Insecure {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	// --- Trace exporter ---
	traceExporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(cfg.Endpoint),
		otlptracegrpc.WithDialOption(dialOpts...),
	)
	if err != nil {
		return nil, err
	}

	sampler := sdktrace.TraceIDRatioBased(cfg.SampleRate)
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)
	otel.SetTracerProvider(tp)

	// --- Metric exporter ---
	metricExporter, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithEndpoint(cfg.Endpoint),
		otlpmetricgrpc.WithDialOption(dialOpts...),
	)
	if err != nil {
		// Shut down the already-initialized trace provider before returning.
		_ = tp.Shutdown(ctx)
		return nil, err
	}

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter)),
		sdkmetric.WithResource(res),
	)
	otel.SetMeterProvider(mp)

	slog.Info("otel: initialized",
		"endpoint", cfg.Endpoint,
		"service", cfg.ServiceName,
		"sample_rate", cfg.SampleRate,
	)

	shutdown := func(ctx context.Context) error {
		slog.Info("otel: shutting down providers")
		return errors.Join(tp.Shutdown(ctx), mp.Shutdown(ctx))
	}

	return shutdown, nil
}
