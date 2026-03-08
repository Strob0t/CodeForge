package otel

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestInitTracerDisabled(t *testing.T) {
	t.Parallel()

	cfg := OTELConfig{
		Enabled: false,
	}

	shutdown, err := InitTracer(cfg)
	if err != nil {
		t.Fatalf("InitTracer() error = %v", err)
	}
	if shutdown == nil {
		t.Fatal("InitTracer() returned nil shutdown function")
	}

	// Shutdown should be a no-op and not error.
	if err := shutdown(context.Background()); err != nil {
		t.Errorf("shutdown() error = %v, want nil", err)
	}
}

func TestOTELConfigFields(t *testing.T) {
	t.Parallel()

	cfg := OTELConfig{
		Enabled:     true,
		Endpoint:    "localhost:4317",
		ServiceName: "codeforge",
		Insecure:    true,
		SampleRate:  0.5,
	}

	if !cfg.Enabled {
		t.Error("Enabled = false, want true")
	}
	if cfg.Endpoint != "localhost:4317" {
		t.Errorf("Endpoint = %q, want %q", cfg.Endpoint, "localhost:4317")
	}
	if cfg.ServiceName != "codeforge" {
		t.Errorf("ServiceName = %q, want %q", cfg.ServiceName, "codeforge")
	}
	if !cfg.Insecure {
		t.Error("Insecure = false, want true")
	}
	if cfg.SampleRate != 0.5 {
		t.Errorf("SampleRate = %f, want 0.5", cfg.SampleRate)
	}
}

func TestHTTPMiddleware(t *testing.T) {
	t.Parallel()

	middleware := HTTPMiddleware("test-service")
	if middleware == nil {
		t.Fatal("HTTPMiddleware() returned nil")
	}

	// Verify the middleware wraps a handler and produces a valid response.
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	handler := middleware(inner)
	if handler == nil {
		t.Fatal("middleware(inner) returned nil")
	}

	req := httptest.NewRequest(http.MethodGet, "/test", http.NoBody)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("response status = %d, want %d", rec.Code, http.StatusOK)
	}
	if rec.Body.String() != "ok" {
		t.Errorf("response body = %q, want %q", rec.Body.String(), "ok")
	}
}

func TestStartRunSpan(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	newCtx, span := StartRunSpan(ctx, "run-1", "task-1", "proj-1")

	if newCtx == nil {
		t.Fatal("StartRunSpan() returned nil context")
	}
	if span == nil {
		t.Fatal("StartRunSpan() returned nil span")
	}

	// End the span to avoid leaks.
	span.End()
}

func TestStartToolCallSpan(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	newCtx, span := StartToolCallSpan(ctx, "call-1", "read_file")

	if newCtx == nil {
		t.Fatal("StartToolCallSpan() returned nil context")
	}
	if span == nil {
		t.Fatal("StartToolCallSpan() returned nil span")
	}

	span.End()
}

func TestStartDeliverySpan(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	newCtx, span := StartDeliverySpan(ctx, "run-1", "pr")

	if newCtx == nil {
		t.Fatal("StartDeliverySpan() returned nil context")
	}
	if span == nil {
		t.Fatal("StartDeliverySpan() returned nil span")
	}

	span.End()
}

func TestNewMetrics(t *testing.T) {
	t.Parallel()

	m, err := NewMetrics()
	if err != nil {
		t.Fatalf("NewMetrics() error = %v", err)
	}
	if m == nil {
		t.Fatal("NewMetrics() returned nil")
	}

	if m.RunsStarted == nil {
		t.Error("RunsStarted is nil")
	}
	if m.RunsCompleted == nil {
		t.Error("RunsCompleted is nil")
	}
	if m.RunsFailed == nil {
		t.Error("RunsFailed is nil")
	}
	if m.ToolCalls == nil {
		t.Error("ToolCalls is nil")
	}
	if m.RunDuration == nil {
		t.Error("RunDuration is nil")
	}
	if m.RunCost == nil {
		t.Error("RunCost is nil")
	}
}

func TestMetricsRecordDoesNotPanic(t *testing.T) {
	t.Parallel()

	m, err := NewMetrics()
	if err != nil {
		t.Fatalf("NewMetrics() error = %v", err)
	}

	ctx := context.Background()

	// These should not panic even with no-op providers.
	m.RunsStarted.Add(ctx, 1)
	m.RunsCompleted.Add(ctx, 1)
	m.RunsFailed.Add(ctx, 1)
	m.ToolCalls.Add(ctx, 5)
	m.RunDuration.Record(ctx, 1.5)
	m.RunCost.Record(ctx, 0.003)
}

func TestShutdownFuncType(t *testing.T) {
	t.Parallel()

	// Verify the ShutdownFunc type is callable.
	var fn ShutdownFunc = func(_ context.Context) error { return nil }
	if err := fn(context.Background()); err != nil {
		t.Errorf("ShutdownFunc() = %v, want nil", err)
	}
}
