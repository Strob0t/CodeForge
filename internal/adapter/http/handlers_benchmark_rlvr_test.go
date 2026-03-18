package http_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/benchmark"
)

func TestExportRLVRData_JSONL(t *testing.T) {
	// DevModeOnly middleware checks APP_ENV.
	t.Setenv("APP_ENV", "development")

	store := &mockStore{
		benchmarkRuns: []benchmark.Run{
			{ID: "run-rlvr-1", Model: "gpt-4", Status: benchmark.StatusCompleted},
		},
		benchmarkResults: []benchmark.Result{
			{
				RunID:        "run-rlvr-1",
				TaskID:       "t1",
				TaskName:     "Fix bug",
				ActualOutput: "fixed code",
				Scores:       json.RawMessage(`{"correctness":0.8}`),
			},
		},
	}
	r := newTestRouterWithStore(store)

	req := httptest.NewRequest("GET", "/api/v1/benchmarks/runs/run-rlvr-1/export/rlvr", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	ct := w.Header().Get("Content-Type")
	if ct != "application/x-ndjson" {
		t.Errorf("Content-Type = %q, want application/x-ndjson", ct)
	}

	// Parse JSONL output.
	var entry benchmark.RLVREntry
	if err := json.NewDecoder(w.Body).Decode(&entry); err != nil {
		t.Fatalf("failed to decode JSONL line: %v", err)
	}
	if entry.Prompt != "Fix bug" {
		t.Errorf("Prompt = %q, want %q", entry.Prompt, "Fix bug")
	}
	if entry.Response != "fixed code" {
		t.Errorf("Response = %q, want %q", entry.Response, "fixed code")
	}
	if entry.Reward < 0.79 || entry.Reward > 0.81 {
		t.Errorf("Reward = %f, want ~0.8", entry.Reward)
	}
	if entry.Metadata["task_id"] != "t1" {
		t.Errorf("metadata.task_id = %q, want t1", entry.Metadata["task_id"])
	}
}

func TestExportRLVRData_JSON(t *testing.T) {
	t.Setenv("APP_ENV", "development")

	store := &mockStore{
		benchmarkRuns: []benchmark.Run{
			{ID: "run-rlvr-2", Model: "claude-3", Status: benchmark.StatusCompleted},
		},
		benchmarkResults: []benchmark.Result{
			{
				RunID:    "run-rlvr-2",
				TaskID:   "t1",
				TaskName: "Sort array",
				Scores:   json.RawMessage(`{"functional_test":1.0}`),
			},
		},
	}
	r := newTestRouterWithStore(store)

	req := httptest.NewRequest("GET", "/api/v1/benchmarks/runs/run-rlvr-2/export/rlvr?format=json", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var entries []benchmark.RLVREntry
	if err := json.NewDecoder(w.Body).Decode(&entries); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Metadata["model"] != "claude-3" {
		t.Errorf("metadata.model = %q, want claude-3", entries[0].Metadata["model"])
	}
}

func TestExportRLVRData_EmptyResults(t *testing.T) {
	t.Setenv("APP_ENV", "development")

	store := &mockStore{
		benchmarkRuns: []benchmark.Run{
			{ID: "run-rlvr-3", Model: "gpt-4"},
		},
	}
	r := newTestRouterWithStore(store)

	req := httptest.NewRequest("GET", "/api/v1/benchmarks/runs/run-rlvr-3/export/rlvr?format=json", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var entries []benchmark.RLVREntry
	if err := json.NewDecoder(w.Body).Decode(&entries); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(entries))
	}
}

func TestExportRLVRData_DevModeRequired(t *testing.T) {
	// Ensure APP_ENV is NOT development.
	t.Setenv("APP_ENV", "")

	r := newTestRouter()
	req := httptest.NewRequest("GET", "/api/v1/benchmarks/runs/run-1/export/rlvr", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 without APP_ENV=development, got %d", w.Code)
	}
}
