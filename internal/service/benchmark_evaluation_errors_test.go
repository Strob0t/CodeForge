package service_test

import (
	"context"
	"encoding/json"
	"math"
	"sync"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/benchmark"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
)

// benchEventHub records the benchmark.task.completed scores.
type benchEventHub struct {
	mu     sync.Mutex
	scores []float64
}

func (h *benchEventHub) BroadcastEvent(_ context.Context, eventType string, payload any) {
	if eventType != "benchmark.task.completed" {
		return
	}
	if p, ok := payload.(service.BenchmarkTaskCompletedPayload); ok {
		h.mu.Lock()
		h.scores = append(h.scores, p.Score)
		h.mu.Unlock()
	}
}

// S6-G review, item 3 (and S6-H follow-up a): evaluator errors travel in
// their own field end to end and never count as a 0.0 score. The Go averages
// also skip `<evaluator>_error` keys a worker might still send as scores.
func TestBenchmarkRunResult_EvaluationErrorsAreNotScores(t *testing.T) {
	store := newBenchMockStore()
	store.benchRuns["run-ee"] = &benchmark.Run{ID: "run-ee", Status: benchmark.StatusRunning}
	svc := newTestBenchmarkService(store)
	hub := &benchEventHub{}
	svc.SetHub(hub)

	payload, err := json.Marshal(messagequeue.BenchmarkRunResultPayload{
		RunID:  "run-ee",
		Status: "completed",
		Results: []messagequeue.BenchmarkTaskResult{{
			TaskID:   "t1",
			TaskName: "task",
			// A defensive case: an old worker still sends the error marker as a score.
			Scores:           map[string]float64{"correctness": 0.8, "trajectory_error_recovery": 0.6, "llm_judge_error": 0},
			EvaluationErrors: map[string]string{"llm_judge_error": "proxy down"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.HandleBenchmarkRunResult(context.Background(), messagequeue.SubjectBenchmarkRunResult, payload); err != nil {
		t.Fatalf("HandleBenchmarkRunResult: %v", err)
	}

	stored := store.benchResult["run-ee"]
	if len(stored) != 1 {
		t.Fatalf("stored %d results, want 1", len(stored))
	}
	if got := stored[0].EvaluationErrors; len(got) != 1 || got["llm_judge_error"] != "proxy down" {
		t.Fatalf("stored evaluation errors = %v, want the llm_judge error", got)
	}

	const want = 0.7 // correctness and trajectory_error_recovery; the error marker is no score
	if len(hub.scores) != 1 || math.Abs(hub.scores[0]-want) > 1e-9 {
		t.Fatalf("task.completed scores = %v, want [%v]", hub.scores, want)
	}
	analysis, err := svc.CostAnalysis(context.Background(), "run-ee")
	if err != nil {
		t.Fatalf("CostAnalysis: %v", err)
	}
	if len(analysis.TaskBreakdown) != 1 || math.Abs(analysis.TaskBreakdown[0].Score-want) > 1e-9 {
		t.Fatalf("cost analysis breakdown = %+v, want score %v", analysis.TaskBreakdown, want)
	}
}
