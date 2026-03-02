package http

import (
	"net/http"

	"github.com/Strob0t/CodeForge/internal/domain/benchmark"
)

// --- Suite CRUD (Phase 26) ---

// ListBenchmarkSuites handles GET /api/v1/benchmarks/suites
func (h *Handlers) ListBenchmarkSuites(w http.ResponseWriter, r *http.Request) {
	handleList(h.Benchmarks.ListSuites)(w, r)
}

// CreateBenchmarkSuite handles POST /api/v1/benchmarks/suites
func (h *Handlers) CreateBenchmarkSuite(w http.ResponseWriter, r *http.Request) {
	req, ok := readJSON[benchmark.CreateSuiteRequest](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	suite, err := h.Benchmarks.RegisterSuite(r.Context(), &req)
	if err != nil {
		writeDomainError(w, err, "create benchmark suite")
		return
	}
	writeJSON(w, http.StatusCreated, suite)
}

// GetBenchmarkSuite handles GET /api/v1/benchmarks/suites/{id}
func (h *Handlers) GetBenchmarkSuite(w http.ResponseWriter, r *http.Request) {
	handleGet(h.Benchmarks.GetSuite, "benchmark suite not found")(w, r)
}

// DeleteBenchmarkSuite handles DELETE /api/v1/benchmarks/suites/{id}
func (h *Handlers) DeleteBenchmarkSuite(w http.ResponseWriter, r *http.Request) {
	handleDelete(h.Benchmarks.DeleteSuite, "benchmark suite not found")(w, r)
}

// --- Run CRUD ---

// ListBenchmarkRuns handles GET /api/v1/benchmarks/runs
func (h *Handlers) ListBenchmarkRuns(w http.ResponseWriter, r *http.Request) {
	suiteID := r.URL.Query().Get("suite_id")
	benchType := r.URL.Query().Get("benchmark_type")
	model := r.URL.Query().Get("model")

	if suiteID != "" || benchType != "" || model != "" {
		filter := benchmark.RunFilter{
			SuiteID:       suiteID,
			BenchmarkType: benchmark.BenchmarkType(benchType),
			Model:         model,
		}
		runs, err := h.Benchmarks.ListRunsFiltered(r.Context(), filter)
		if err != nil {
			writeInternalError(w, err)
			return
		}
		if runs == nil {
			runs = []benchmark.Run{}
		}
		writeJSON(w, http.StatusOK, runs)
		return
	}
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

// --- Results & Comparison ---

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
