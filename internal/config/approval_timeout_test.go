package config

import (
	"testing"
	"time"
)

// TestRuntimeApprovalTimeout: one value decides how long the Go Core waits for
// a HITL decision and how long the worker waits for the policy response (KI-21).
func TestRuntimeApprovalTimeout(t *testing.T) {
	tests := []struct {
		name string
		cfg  *Runtime
		want time.Duration
	}{
		{name: "nil config uses the default", cfg: nil, want: 60 * time.Second},
		{name: "unset uses the default", cfg: &Runtime{}, want: 60 * time.Second},
		{name: "negative uses the default", cfg: &Runtime{ApprovalTimeoutSeconds: -5}, want: 60 * time.Second},
		{name: "one second", cfg: &Runtime{ApprovalTimeoutSeconds: 1}, want: time.Second},
		{name: "configured", cfg: &Runtime{ApprovalTimeoutSeconds: 300}, want: 300 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.ApprovalTimeout(); got != tt.want {
				t.Fatalf("ApprovalTimeout() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDefaultsApprovalTimeout(t *testing.T) {
	cfg := Defaults()
	if got := cfg.Runtime.ApprovalTimeout(); got != 60*time.Second {
		t.Fatalf("default approval timeout = %v, want 60s", got)
	}
}
