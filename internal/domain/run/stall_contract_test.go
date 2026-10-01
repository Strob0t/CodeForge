package run_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/run"
)

// S6-F 8: a run is stalled whether the Go Core stopped it (StallDetectedError)
// or the worker's agent loop aborted it ("stall detected: repeated X after N
// escape attempts"). Both start with one marker; testdata/stall_contract.json
// is the contract the worker's test (workers/tests/test_stall_contract.py)
// checks too.

type stallContract struct {
	Marker      string `json:"marker"`
	WorkerError string `json:"worker_error"`
}

func loadStallContract(t *testing.T) stallContract {
	t.Helper()
	data, err := os.ReadFile("testdata/stall_contract.json")
	if err != nil {
		t.Fatalf("read contract: %v", err)
	}
	var c stallContract
	if err := json.Unmarshal(data, &c); err != nil {
		t.Fatalf("parse contract: %v", err)
	}
	return c
}

func TestStallMarker_MatchesTheContract(t *testing.T) {
	c := loadStallContract(t)
	if run.StallMarker != c.Marker {
		t.Fatalf("StallMarker = %q, contract marker %q", run.StallMarker, c.Marker)
	}
}

func TestRun_Stalled(t *testing.T) {
	c := loadStallContract(t)
	tests := []struct {
		name   string
		status run.Status
		err    string
		want   bool
	}{
		{"go core stall", run.StatusFailed, run.StallDetectedError, true},
		{"worker stall", run.StatusFailed, c.WorkerError, true},
		{"other failure", run.StatusFailed, "iteration limit reached (50)", false},
		{"marker not at the start", run.StatusFailed, "tool failed: stall detected: no", false},
		{"no error", run.StatusFailed, "", false},
		{"stall error but cancelled", run.StatusCancelled, c.WorkerError, false},
		{"stall error but completed", run.StatusCompleted, run.StallDetectedError, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &run.Run{Status: tt.status, Error: tt.err}
			if got := r.Stalled(); got != tt.want {
				t.Fatalf("Stalled() = %v, want %v", got, tt.want)
			}
		})
	}
}
