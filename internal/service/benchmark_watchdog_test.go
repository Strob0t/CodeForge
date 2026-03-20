package service

import (
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/benchmark"
)

func TestWatchdogTimeoutForType(t *testing.T) {
	globalDefault := 2 * time.Hour

	tests := []struct {
		name     string
		bt       benchmark.BenchmarkType
		expected time.Duration
	}{
		{name: "simple returns 30 min", bt: benchmark.TypeSimple, expected: 30 * time.Minute},
		{name: "tool_use returns 60 min", bt: benchmark.TypeToolUse, expected: 60 * time.Minute},
		{name: "agent returns 4 hours", bt: benchmark.TypeAgent, expected: 4 * time.Hour},
		{name: "empty string returns global default", bt: "", expected: globalDefault},
		{name: "unknown type returns global default", bt: "unknown_type", expected: globalDefault},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := watchdogTimeoutForType(tt.bt, globalDefault)
			if got != tt.expected {
				t.Errorf("watchdogTimeoutForType(%q, %s) = %s, want %s", tt.bt, globalDefault, got, tt.expected)
			}
		})
	}
}
