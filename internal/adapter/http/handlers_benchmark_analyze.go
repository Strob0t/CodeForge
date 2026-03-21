package http

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/Strob0t/CodeForge/internal/domain/benchmark"
)

// AnalyzeBenchmarkRun handles POST /api/v1/benchmarks/runs/{id}/analyze.
func (h *Handlers) AnalyzeBenchmarkRun(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "run id required")
		return
	}
	run, err := h.Benchmarks.GetRun(r.Context(), id)
	if err != nil {
		writeDomainError(w, err, "run not found")
		return
	}
	results, err := h.Benchmarks.ListResults(r.Context(), id)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	failed := 0
	for i := range results {
		var scores map[string]float64
		_ = json.Unmarshal(results[i].Scores, &scores)
		if len(scores) == 0 {
			failed++
			continue
		}
		var total float64
		for _, v := range scores {
			total += v
		}
		if total/float64(len(scores)) < 0.5 {
			failed++
		}
	}
	failureRate := 0.0
	if len(results) > 0 {
		failureRate = float64(failed) / float64(len(results))
	}
	report := map[string]any{
		"run_id":               id,
		"mode":                 "coder",
		"model_family":         benchmark.ModelFamily(run.Model),
		"total_tasks":          len(results),
		"failed_tasks":         failed,
		"failure_rate":         failureRate,
		"tactical_fixes":       []any{},
		"strategic_principles": []string{},
	}
	writeJSON(w, http.StatusOK, report)
}
