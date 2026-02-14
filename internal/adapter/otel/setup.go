// Package otel provides a stub for OpenTelemetry tracing setup.
// This will be implemented in Phase 2 to provide distributed tracing.
package otel

import (
	"context"
	"log/slog"
)

// ShutdownFunc is called to flush and shut down the trace provider.
type ShutdownFunc func(ctx context.Context) error

// InitTracer returns a no-op shutdown function.
// In Phase 2, this will initialize an OTLP exporter and TracerProvider.
func InitTracer(serviceName string) ShutdownFunc {
	slog.Info("otel stub: InitTracer called", "service", serviceName)
	return func(_ context.Context) error {
		slog.Info("otel stub: shutdown called")
		return nil
	}
}
