package postgres_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/Strob0t/CodeForge/internal/domain/benchmark"
)

// S6-G review, item 3: a result's evaluation errors are stored apart from its
// scores and read back; a result without errors reads back none.
func TestStore_BenchmarkResultEvaluationErrors(t *testing.T) {
	store := setupStore(t)
	tenantID := createTestTenant(t, store)
	ctx := ctxWithTenant(t, tenantID)

	run := &benchmark.Run{Dataset: "basic-coding", Model: "openai/gpt-4o", Metrics: []string{"llm_judge"}, Status: benchmark.StatusRunning}
	if err := store.CreateBenchmarkRun(ctx, run); err != nil {
		t.Fatalf("CreateBenchmarkRun: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteBenchmarkRun(ctx, run.ID) })

	withErrors := &benchmark.Result{
		ID: uuid.New().String(), RunID: run.ID, TaskID: "t1", TaskName: "with errors",
		Scores:           json.RawMessage(`{"correctness":0.8}`),
		EvaluationErrors: map[string]string{"llm_judge_error": "proxy down"},
	}
	clean := &benchmark.Result{ID: uuid.New().String(), RunID: run.ID, TaskID: "t2", TaskName: "clean", Scores: json.RawMessage(`{"correctness":1}`)}
	for _, r := range []*benchmark.Result{withErrors, clean} {
		if err := store.CreateBenchmarkResult(ctx, r); err != nil {
			t.Fatalf("CreateBenchmarkResult: %v", err)
		}
	}

	results, err := store.ListBenchmarkResults(ctx, run.ID)
	if err != nil {
		t.Fatalf("ListBenchmarkResults: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	if got := results[0].EvaluationErrors; len(got) != 1 || got["llm_judge_error"] != "proxy down" {
		t.Errorf("t1 evaluation errors = %v, want the llm_judge error", got)
	}
	if got := results[1].EvaluationErrors; got != nil {
		t.Errorf("t2 evaluation errors = %v, want none", got)
	}
}
