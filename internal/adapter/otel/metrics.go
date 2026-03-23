package otel

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	cfmetrics "github.com/Strob0t/CodeForge/internal/port/metrics"
)

// Compile-time assertion: *Metrics satisfies metrics.Recorder.
var _ cfmetrics.Recorder = (*Metrics)(nil)

const meterName = "codeforge"

// Metrics holds all CodeForge metric instruments.
type Metrics struct {
	RunsStarted   metric.Int64Counter
	RunsCompleted metric.Int64Counter
	RunsFailed    metric.Int64Counter
	ToolCalls     metric.Int64Counter
	RunDuration   metric.Float64Histogram
	RunCost       metric.Float64Histogram
}

// NewMetrics creates all metric instruments.
func NewMetrics() (*Metrics, error) {
	meter := otel.Meter(meterName)
	m := &Metrics{}
	var err error

	m.RunsStarted, err = meter.Int64Counter("codeforge.runs.started",
		metric.WithDescription("Number of runs started"))
	if err != nil {
		return nil, err
	}

	m.RunsCompleted, err = meter.Int64Counter("codeforge.runs.completed",
		metric.WithDescription("Number of runs completed"))
	if err != nil {
		return nil, err
	}

	m.RunsFailed, err = meter.Int64Counter("codeforge.runs.failed",
		metric.WithDescription("Number of runs failed"))
	if err != nil {
		return nil, err
	}

	m.ToolCalls, err = meter.Int64Counter("codeforge.toolcalls",
		metric.WithDescription("Number of tool calls"))
	if err != nil {
		return nil, err
	}

	m.RunDuration, err = meter.Float64Histogram("codeforge.run.duration_seconds",
		metric.WithDescription("Run duration in seconds"))
	if err != nil {
		return nil, err
	}

	m.RunCost, err = meter.Float64Histogram("codeforge.run.cost_usd",
		metric.WithDescription("Run cost in USD"))
	if err != nil {
		return nil, err
	}

	return m, nil
}

// --- port/metrics.Recorder implementation ---
//
// These methods wrap the direct OTEL instrument access with a simple
// key-value string API. Attribute pairs are passed as alternating
// key-value strings: RecordRunStarted(ctx, "project.id", "p1", "type", "agentic").

// attrsFromPairs converts alternating key-value strings to OTEL attributes.
func attrsFromPairs(pairs []string) metric.MeasurementOption {
	kvs := make([]attribute.KeyValue, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		kvs = append(kvs, attribute.String(pairs[i], pairs[i+1]))
	}
	return metric.WithAttributes(kvs...)
}

// RecordRunStarted increments the runs started counter.
func (m *Metrics) RecordRunStarted(ctx context.Context, attrs ...string) {
	m.RunsStarted.Add(ctx, 1, attrsFromPairs(attrs))
}

// RecordRunCompleted increments the runs completed counter.
func (m *Metrics) RecordRunCompleted(ctx context.Context, attrs ...string) {
	m.RunsCompleted.Add(ctx, 1, attrsFromPairs(attrs))
}

// RecordRunFailed increments the runs failed counter.
func (m *Metrics) RecordRunFailed(ctx context.Context, attrs ...string) {
	m.RunsFailed.Add(ctx, 1, attrsFromPairs(attrs))
}

// RecordToolCall increments the tool calls counter.
func (m *Metrics) RecordToolCall(ctx context.Context, attrs ...string) {
	m.ToolCalls.Add(ctx, 1, attrsFromPairs(attrs))
}

// RecordRunDuration records a run duration observation in seconds.
func (m *Metrics) RecordRunDuration(ctx context.Context, seconds float64, attrs ...string) {
	m.RunDuration.Record(ctx, seconds, attrsFromPairs(attrs))
}

// RecordRunCost records a run cost observation in USD.
func (m *Metrics) RecordRunCost(ctx context.Context, cost float64, attrs ...string) {
	m.RunCost.Record(ctx, cost, attrsFromPairs(attrs))
}
