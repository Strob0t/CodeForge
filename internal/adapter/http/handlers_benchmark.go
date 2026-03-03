package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

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
	status := r.URL.Query().Get("status")
	sort := r.URL.Query().Get("sort")

	if suiteID != "" || benchType != "" || model != "" || status != "" || sort != "" {
		filter := benchmark.RunFilter{
			SuiteID:       suiteID,
			BenchmarkType: benchmark.BenchmarkType(benchType),
			Model:         model,
			Status:        benchmark.RunStatus(status),
			Sort:          sort,
		}
		runs, err := h.Benchmarks.ListRunsFiltered(r.Context(), &filter)
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
// Creates the run in the database and dispatches it to the Python worker via NATS.
func (h *Handlers) CreateBenchmarkRun(w http.ResponseWriter, r *http.Request) {
	req, ok := readJSON[benchmark.CreateRunRequest](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	run, err := h.Benchmarks.StartRun(r.Context(), &req)
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

// CancelBenchmarkRun handles PATCH /api/v1/benchmarks/runs/{id}
// Sets the run status to "failed" (cancelled).
func (h *Handlers) CancelBenchmarkRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "run id is required")
		return
	}
	run, err := h.Benchmarks.GetRun(r.Context(), id)
	if err != nil {
		writeDomainError(w, err, "benchmark run not found")
		return
	}
	if run.Status != benchmark.StatusRunning {
		writeError(w, http.StatusBadRequest, "run is not in running state")
		return
	}
	run.Status = benchmark.StatusFailed
	if err := h.Benchmarks.UpdateRun(r.Context(), run); err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, run)
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

// ExportBenchmarkResults handles GET /api/v1/benchmarks/runs/{id}/export/results
// Exports results as JSON or CSV.
func (h *Handlers) ExportBenchmarkResults(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "run id is required")
		return
	}
	results, err := h.Benchmarks.ListResults(r.Context(), id)
	if err != nil {
		writeDomainError(w, err, "benchmark results not found")
		return
	}

	format := strings.ToLower(r.URL.Query().Get("format"))
	if format == "csv" {
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", "attachment; filename=\"results.csv\"")
		_, _ = w.Write([]byte("task_id,task_name,cost_usd,tokens_in,tokens_out,duration_ms\n"))
		for i := range results {
			line := fmt.Sprintf("%s,%s,%.6f,%d,%d,%d\n",
				results[i].TaskID, results[i].TaskName, results[i].CostUSD,
				results[i].TokensIn, results[i].TokensOut, results[i].DurationMs)
			_, _ = w.Write([]byte(line)) //nolint:gosec // CSV export, not HTML
		}
		return
	}

	writeJSON(w, http.StatusOK, results)
}

// UpdateBenchmarkSuite handles PUT /api/v1/benchmarks/suites/{id}
func (h *Handlers) UpdateBenchmarkSuite(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "suite id is required")
		return
	}
	suite, err := h.Benchmarks.GetSuite(r.Context(), id)
	if err != nil {
		writeDomainError(w, err, "benchmark suite not found")
		return
	}

	req, ok := readJSON[benchmark.CreateSuiteRequest](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}

	// Update mutable fields
	if req.Name != "" {
		suite.Name = req.Name
	}
	if req.Description != "" {
		suite.Description = req.Description
	}
	if req.Type != "" {
		suite.Type = req.Type
	}
	if req.ProviderName != "" {
		suite.ProviderName = req.ProviderName
	}
	if req.Config != nil {
		suite.Config = req.Config
	}

	if err := h.Benchmarks.UpdateSuite(r.Context(), suite); err != nil {
		writeDomainError(w, err, "update benchmark suite")
		return
	}
	writeJSON(w, http.StatusOK, suite)
}

// --- Phase 26G: Multi-Compare, Cost Analysis, Leaderboard ---

// MultiCompareBenchmarkRuns handles POST /api/v1/benchmarks/compare-multi
func (h *Handlers) MultiCompareBenchmarkRuns(w http.ResponseWriter, r *http.Request) {
	req, ok := readJSON[benchmark.MultiCompareRequest](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	if len(req.RunIDs) < 2 {
		writeError(w, http.StatusBadRequest, "at least 2 run_ids are required")
		return
	}
	entries, err := h.Benchmarks.CompareMulti(r.Context(), req.RunIDs)
	if err != nil {
		writeDomainError(w, err, "multi-compare benchmark runs")
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

// BenchmarkCostAnalysis handles GET /api/v1/benchmarks/runs/{id}/cost-analysis
func (h *Handlers) BenchmarkCostAnalysis(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "run id is required")
		return
	}
	analysis, err := h.Benchmarks.CostAnalysis(r.Context(), id)
	if err != nil {
		writeDomainError(w, err, "benchmark cost analysis")
		return
	}
	writeJSON(w, http.StatusOK, analysis)
}

// BenchmarkLeaderboard handles GET /api/v1/benchmarks/leaderboard
func (h *Handlers) BenchmarkLeaderboard(w http.ResponseWriter, r *http.Request) {
	suiteID := r.URL.Query().Get("suite_id")
	entries, err := h.Benchmarks.Leaderboard(r.Context(), suiteID)
	if err != nil {
		writeDomainError(w, err, "benchmark leaderboard")
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

// --- Phase 28E: Training Data Export ---

// ExportTrainingData handles GET /api/v1/benchmarks/runs/{id}/export/training
func (h *Handlers) ExportTrainingData(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "run id is required")
		return
	}
	pairs, err := h.Benchmarks.ExportTrainingPairs(r.Context(), id)
	if err != nil {
		writeDomainError(w, err, "export training data")
		return
	}

	format := strings.ToLower(r.URL.Query().Get("format"))
	if format == "json" {
		writeJSON(w, http.StatusOK, pairs)
		return
	}

	// Default: JSONL (ndjson) — one JSON object per line.
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Content-Disposition", "attachment; filename=\"training_pairs.jsonl\"")
	enc := json.NewEncoder(w)
	for i := range pairs {
		_ = enc.Encode(pairs[i])
	}
}
