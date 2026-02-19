package otel

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "codeforge"

// StartRunSpan starts a span for an agent run.
func StartRunSpan(ctx context.Context, runID, taskID, projectID string) (context.Context, trace.Span) {
	return otel.Tracer(tracerName).Start(ctx, "run",
		trace.WithAttributes(
			attribute.String("run.id", runID),
			attribute.String("task.id", taskID),
			attribute.String("project.id", projectID),
		),
	)
}

// StartToolCallSpan starts a span for a tool call within a run.
func StartToolCallSpan(ctx context.Context, callID, tool string) (context.Context, trace.Span) {
	return otel.Tracer(tracerName).Start(ctx, "toolcall",
		trace.WithAttributes(
			attribute.String("toolcall.id", callID),
			attribute.String("toolcall.tool", tool),
		),
	)
}

// StartDeliverySpan starts a span for output delivery.
func StartDeliverySpan(ctx context.Context, runID, mode string) (context.Context, trace.Span) {
	return otel.Tracer(tracerName).Start(ctx, "delivery",
		trace.WithAttributes(
			attribute.String("run.id", runID),
			attribute.String("delivery.mode", mode),
		),
	)
}
