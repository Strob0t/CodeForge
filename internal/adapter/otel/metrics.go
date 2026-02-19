package otel

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

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
