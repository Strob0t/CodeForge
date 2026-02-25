package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/Strob0t/CodeForge/internal/domain/benchmark"
)

// ListBenchmarkRuns handles GET /api/v1/benchmarks/runs
func (h *Handlers) ListBenchmarkRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := h.Benchmarks.ListRuns(r.Context())
	if err != nil {
		writeInternalError(w, err)
		return
	}
	if runs == nil {
		runs = []benchmark.Run{}
	}
	writeJSON(w, http.StatusOK, runs)
}

// GetBenchmarkRun handles GET /api/v1/benchmarks/runs/{id}
func (h *Handlers) GetBenchmarkRun(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	run, err := h.Benchmarks.GetRun(r.Context(), id)
	if err != nil {
		writeDomainError(w, err, "benchmark run not found")
		return
	}
	writeJSON(w, http.StatusOK, run)
}

// CreateBenchmarkRun handles POST /api/v1/benchmarks/runs
func (h *Handlers) CreateBenchmarkRun(w http.ResponseWriter, r *http.Request) {
	req, ok := readJSON[benchmark.CreateRunRequest](w, r)
	if !ok {
		return
	}
	run, err := h.Benchmarks.CreateRun(r.Context(), &req)
	if err != nil {
		writeDomainError(w, err, "create benchmark run")
		return
	}
	writeJSON(w, http.StatusCreated, run)
}

// DeleteBenchmarkRun handles DELETE /api/v1/benchmarks/runs/{id}
func (h *Handlers) DeleteBenchmarkRun(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.Benchmarks.DeleteRun(r.Context(), id); err != nil {
		writeDomainError(w, err, "benchmark run not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListBenchmarkResults handles GET /api/v1/benchmarks/runs/{id}/results
func (h *Handlers) ListBenchmarkResults(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "id")
	results, err := h.Benchmarks.ListResults(r.Context(), runID)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	if results == nil {
		results = []benchmark.Result{}
	}
	writeJSON(w, http.StatusOK, results)
}

// CompareBenchmarkRuns handles POST /api/v1/benchmarks/compare
func (h *Handlers) CompareBenchmarkRuns(w http.ResponseWriter, r *http.Request) {
	req, ok := readJSON[benchmark.CompareRequest](w, r)
	if !ok {
		return
	}
	if req.RunIDA == "" || req.RunIDB == "" {
		writeError(w, http.StatusBadRequest, "run_id_a and run_id_b are required")
		return
	}
	result, err := h.Benchmarks.Compare(r.Context(), req.RunIDA, req.RunIDB)
	if err != nil {
		writeDomainError(w, err, "compare benchmark runs")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// ListBenchmarkDatasets handles GET /api/v1/benchmarks/datasets
func (h *Handlers) ListBenchmarkDatasets(w http.ResponseWriter, r *http.Request) {
	datasets, err := h.Benchmarks.ListDatasets()
	if err != nil {
		writeInternalError(w, err)
		return
	}
	if datasets == nil {
		datasets = []benchmark.DatasetInfo{}
	}
	writeJSON(w, http.StatusOK, datasets)
}
