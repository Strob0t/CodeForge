package service_test

import (
	"context"
	"encoding/json"
	"math"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/benchmark"
	"github.com/Strob0t/CodeForge/internal/service"
)

func TestComputeRLVRReward(t *testing.T) {
	tests := []struct {
		name   string
		scores map[string]float64
		want   float64
	}{
		{
			name:   "single score",
			scores: map[string]float64{"correctness": 0.8},
			want:   0.8,
		},
		{
			name:   "functional_test weighted 2x",
			scores: map[string]float64{"functional_test": 0.9, "correctness": 0.5},
			want:   (0.9*2 + 0.5*1) / 3.0,
		},
		{
			name:   "empty scores returns zero",
			scores: map[string]float64{},
			want:   0.0,
		},
		{
			name:   "nil scores returns zero",
			scores: nil,
			want:   0.0,
		},
		{
			name:   "clamp above 1.0",
			scores: map[string]float64{"correctness": 1.5},
			want:   1.0,
		},
		{
			name:   "clamp below 0.0",
			scores: map[string]float64{"correctness": -0.3},
			want:   0.0,
		},
		{
			name:   "multiple scores without functional_test",
			scores: map[string]float64{"correctness": 0.6, "relevance": 0.8},
			want:   0.7,
		},
		{
			name:   "only functional_test",
			scores: map[string]float64{"functional_test": 0.75},
			want:   0.75,
		},
		{
			name:   "all zero scores",
			scores: map[string]float64{"correctness": 0.0, "functional_test": 0.0},
			want:   0.0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := service.ComputeRLVRReward(tc.scores)
			if math.Abs(got-tc.want) > 1e-9 {
				t.Errorf("ComputeRLVRReward(%v) = %f, want %f", tc.scores, got, tc.want)
			}
		})
	}
}

func TestExportRLVRDataset(t *testing.T) {
	store := newBenchMockStore()
	svc := newTestBenchmarkService(store)
	ctx := context.Background()

	store.benchRuns["run-1"] = &benchmark.Run{
		ID:    "run-1",
		Model: "gpt-4",
	}
	store.benchResult["run-1"] = []benchmark.Result{
		{
			RunID:        "run-1",
			TaskID:       "t1",
			TaskName:     "Fix bug",
			ActualOutput: "fixed code",
			Scores:       json.RawMessage(`{"correctness":0.8}`),
		},
		{
			RunID:        "run-1",
			TaskID:       "t2",
			TaskName:     "Sort array",
			ActualOutput: "sorted code",
			Scores:       json.RawMessage(`{"functional_test":0.9,"correctness":0.5}`),
		},
	}

	entries, err := svc.ExportRLVRDataset(ctx, "run-1")
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	// First entry
	if entries[0].Prompt != "Fix bug" {
		t.Errorf("entries[0].Prompt = %q, want %q", entries[0].Prompt, "Fix bug")
	}
	if entries[0].Response != "fixed code" {
		t.Errorf("entries[0].Response = %q, want %q", entries[0].Response, "fixed code")
	}
	if math.Abs(entries[0].Reward-0.8) > 1e-9 {
		t.Errorf("entries[0].Reward = %f, want 0.8", entries[0].Reward)
	}
	if entries[0].Metadata["task_id"] != "t1" {
		t.Errorf("entries[0].Metadata[task_id] = %q, want t1", entries[0].Metadata["task_id"])
	}
	if entries[0].Metadata["model"] != "gpt-4" {
		t.Errorf("entries[0].Metadata[model] = %q, want gpt-4", entries[0].Metadata["model"])
	}
	if entries[0].Metadata["run_id"] != "run-1" {
		t.Errorf("entries[0].Metadata[run_id] = %q, want run-1", entries[0].Metadata["run_id"])
	}

	// Second entry: functional_test weighted
	expectedReward := (0.9*2 + 0.5*1) / 3.0
	if math.Abs(entries[1].Reward-expectedReward) > 1e-9 {
		t.Errorf("entries[1].Reward = %f, want %f", entries[1].Reward, expectedReward)
	}
}

func TestExportRLVRDataset_Empty(t *testing.T) {
	store := newBenchMockStore()
	svc := newTestBenchmarkService(store)
	ctx := context.Background()

	store.benchRuns["run-1"] = &benchmark.Run{
		ID:    "run-1",
		Model: "gpt-4",
	}
	// No results for this run.

	entries, err := svc.ExportRLVRDataset(ctx, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries for empty results, got %d", len(entries))
	}
}

func TestExportRLVRDataset_MissingRun(t *testing.T) {
	store := newBenchMockStore()
	svc := newTestBenchmarkService(store)
	ctx := context.Background()

	_, err := svc.ExportRLVRDataset(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for missing run")
	}
}

func TestExportRLVRDataset_EmptyScores(t *testing.T) {
	store := newBenchMockStore()
	svc := newTestBenchmarkService(store)
	ctx := context.Background()

	store.benchRuns["run-1"] = &benchmark.Run{
		ID:    "run-1",
		Model: "gpt-4",
	}
	store.benchResult["run-1"] = []benchmark.Result{
		{
			RunID:        "run-1",
			TaskID:       "t1",
			TaskName:     "Empty scores task",
			ActualOutput: "output",
			Scores:       json.RawMessage(`{}`),
		},
	}

	entries, err := svc.ExportRLVRDataset(ctx, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Reward != 0.0 {
		t.Errorf("expected reward 0.0 for empty scores, got %f", entries[0].Reward)
	}
}
