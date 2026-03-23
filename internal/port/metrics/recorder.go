// Package metrics defines the port-layer interface for recording application
// metrics. The primary adapter is adapter/otel.Metrics.
package metrics

import "context"

// Recorder abstracts metric recording operations for the service layer.
// Attribute pairs are passed as alternating key-value strings
// (e.g. "model", "gpt-4o", "project_id", "p1").
type Recorder interface {
	RecordRunStarted(ctx context.Context, attrs ...string)
	RecordRunCompleted(ctx context.Context, attrs ...string)
	RecordRunFailed(ctx context.Context, attrs ...string)
	RecordToolCall(ctx context.Context, attrs ...string)
	RecordRunDuration(ctx context.Context, seconds float64, attrs ...string)
	RecordRunCost(ctx context.Context, cost float64, attrs ...string)
}
