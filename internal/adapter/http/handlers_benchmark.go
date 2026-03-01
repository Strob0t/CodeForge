package http

import (
	"net/http"

	"github.com/Strob0t/CodeForge/internal/domain/benchmark"
)

// ListBenchmarkRuns handles GET /api/v1/benchmarks/runs
func (h *Handlers) ListBenchmarkRuns(w http.ResponseWriter, r *http.Request) {
	handleList(h.Benchmarks.ListRuns)(w, r)
}

// GetBenchmarkRun handles GET /api/v1/benchmarks/runs/{id}
func (h *Handlers) GetBenchmarkRun(w http.ResponseWriter, r *http.Request) {
	handleGet(h.Benchmarks.GetRun, "benchmark run not found")(w, r)
}

// CreateBenchmarkRun handles POST /api/v1/benchmarks/runs
func (h *Handlers) CreateBenchmarkRun(w http.ResponseWriter, r *http.Request) {
	req, ok := readJSON[benchmark.CreateRunRequest](w, r, h.Limits.MaxRequestBodySize)
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
	handleDelete(h.Benchmarks.DeleteRun, "benchmark run not found")(w, r)
}

// ListBenchmarkResults handles GET /api/v1/benchmarks/runs/{id}/results
func (h *Handlers) ListBenchmarkResults(w http.ResponseWriter, r *http.Request) {
	handleListByParam("id", h.Benchmarks.ListResults, "benchmark run not found")(w, r)
}

// CompareBenchmarkRuns handles POST /api/v1/benchmarks/compare
func (h *Handlers) CompareBenchmarkRuns(w http.ResponseWriter, r *http.Request) {
	req, ok := readJSON[benchmark.CompareRequest](w, r, h.Limits.MaxRequestBodySize)
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
