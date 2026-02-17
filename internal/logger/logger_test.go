package logger

import (
	"context"
	"testing"

	"github.com/Strob0t/CodeForge/internal/config"
)

func TestNew(t *testing.T) {
	cfg := config.Logging{Level: "debug", Service: "test-svc"}
	l, closer := New(cfg)
	defer closer.Close()
	if l == nil {
		t.Fatal("expected non-nil logger")
	}
}

func TestNewAsync(t *testing.T) {
	cfg := config.Logging{Level: "debug", Service: "test-svc", Async: true}
	l, closer := New(cfg)
	if l == nil {
		t.Fatal("expected non-nil logger")
	}
	closer.Close()
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"debug", "DEBUG"},
		{"info", "INFO"},
		{"warn", "WARN"},
		{"warning", "WARN"},
		{"error", "ERROR"},
		{"unknown", "INFO"},
		{"", "INFO"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseLevel(tt.input).String()
			if got != tt.want {
				t.Errorf("parseLevel(%q) = %s, want %s", tt.input, got, tt.want)
			}
		})
	}
}

func TestRequestIDContext(t *testing.T) {
	ctx := context.Background()

	// Empty context returns empty string
	if got := RequestID(ctx); got != "" {
		t.Errorf("expected empty request ID, got %q", got)
	}

	// Set and retrieve
	ctx = WithRequestID(ctx, "req-123")
	if got := RequestID(ctx); got != "req-123" {
		t.Errorf("expected req-123, got %q", got)
	}
}
