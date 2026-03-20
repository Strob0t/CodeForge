package http_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/benchmark"
)

func TestExportTrainingData_EmptyPairs_JSONL(t *testing.T) {
	// The default JSONL format must return an empty JSON array when there are
	// no training pairs, not an empty body (zero bytes).
	t.Setenv("APP_ENV", "development")

	store := &mockStore{
		benchmarkRuns: []benchmark.Run{
			{ID: "run-train-empty", Model: "gpt-4", Status: benchmark.StatusCompleted},
		},
		// No benchmarkResults → ExportTrainingPairs returns nil slice.
	}
	r := newTestRouterWithStore(store)

	req := httptest.NewRequest("GET", "/api/v1/benchmarks/runs/run-train-empty/export/training", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Body must not be empty — it should contain a valid JSON array.
	body := w.Body.Bytes()
	if len(body) == 0 {
		t.Fatal("expected non-empty body with JSON array, got zero bytes")
	}

	var pairs []benchmark.TrainingPair
	if err := json.Unmarshal(body, &pairs); err != nil {
		t.Fatalf("failed to decode response as JSON array: %v (body: %q)", err, string(body))
	}
	if len(pairs) != 0 {
		t.Fatalf("expected 0 pairs, got %d", len(pairs))
	}
}

func TestExportTrainingData_EmptyPairs_JSON(t *testing.T) {
	// The ?format=json path already uses writeJSON — verify it also returns [].
	t.Setenv("APP_ENV", "development")

	store := &mockStore{
		benchmarkRuns: []benchmark.Run{
			{ID: "run-train-empty-json", Model: "gpt-4", Status: benchmark.StatusCompleted},
		},
	}
	r := newTestRouterWithStore(store)

	req := httptest.NewRequest("GET", "/api/v1/benchmarks/runs/run-train-empty-json/export/training?format=json", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var pairs []benchmark.TrainingPair
	if err := json.NewDecoder(w.Body).Decode(&pairs); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}
	if len(pairs) != 0 {
		t.Fatalf("expected 0 pairs, got %d", len(pairs))
	}
}

func TestExportTrainingData_WithPairs(t *testing.T) {
	t.Setenv("APP_ENV", "development")

	store := &mockStore{
		benchmarkRuns: []benchmark.Run{
			{ID: "run-train-1", Model: "gpt-4", Status: benchmark.StatusCompleted},
		},
		benchmarkResults: []benchmark.Result{
			{
				RunID:         "run-train-1",
				TaskID:        "t1",
				TaskName:      "Fix bug",
				ActualOutput:  "chosen output",
				Scores:        json.RawMessage(`{"correctness":0.9}`),
				IsBestRollout: true,
				RolloutID:     1,
			},
			{
				RunID:        "run-train-1",
				TaskID:       "t1",
				TaskName:     "Fix bug",
				ActualOutput: "rejected output",
				Scores:       json.RawMessage(`{"correctness":0.3}`),
				RolloutID:    2,
			},
		},
	}
	r := newTestRouterWithStore(store)

	req := httptest.NewRequest("GET", "/api/v1/benchmarks/runs/run-train-1/export/training?format=json", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var pairs []benchmark.TrainingPair
	if err := json.NewDecoder(w.Body).Decode(&pairs); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}
	if len(pairs) != 1 {
		t.Fatalf("expected 1 pair, got %d", len(pairs))
	}
	if pairs[0].TaskID != "t1" {
		t.Errorf("TaskID = %q, want t1", pairs[0].TaskID)
	}
	if pairs[0].Chosen.Output != "chosen output" {
		t.Errorf("Chosen.Output = %q, want %q", pairs[0].Chosen.Output, "chosen output")
	}
	if pairs[0].Rejected.Output != "rejected output" {
		t.Errorf("Rejected.Output = %q, want %q", pairs[0].Rejected.Output, "rejected output")
	}
}

func TestExportTrainingData_DevModeRequired(t *testing.T) {
	t.Setenv("APP_ENV", "")

	r := newTestRouter()
	req := httptest.NewRequest("GET", "/api/v1/benchmarks/runs/run-1/export/training", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 without APP_ENV=development, got %d", w.Code)
	}
}
