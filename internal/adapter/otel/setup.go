// Package otel provides OpenTelemetry tracing and metrics setup for CodeForge.
package otel

import (
	"context"
	"errors"
	"log/slog"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
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

	// The exporters add their own transport credentials after any dial
	// option (TLS unless WithInsecure), so plaintext must be requested with
	// their WithInsecure option.
	traceOpts := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(cfg.Endpoint)}
	metricOpts := []otlpmetricgrpc.Option{otlpmetricgrpc.WithEndpoint(cfg.Endpoint)}
	if cfg.Insecure {
		traceOpts = append(traceOpts, otlptracegrpc.WithInsecure())
		metricOpts = append(metricOpts, otlpmetricgrpc.WithInsecure())
	}

	// --- Trace exporter ---
	traceExporter, err := otlptracegrpc.New(ctx, traceOpts...)
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
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// --- Metric exporter ---
	metricExporter, err := otlpmetricgrpc.New(ctx, metricOpts...)
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
		"insecure", cfg.Insecure,
		"service", cfg.ServiceName,
		"sample_rate", cfg.SampleRate,
	)

	shutdown := func(ctx context.Context) error {
		slog.Info("otel: shutting down providers")
		return errors.Join(tp.Shutdown(ctx), mp.Shutdown(ctx))
	}

	return shutdown, nil
}
