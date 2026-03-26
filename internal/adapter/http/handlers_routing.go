package http

import (
	"net/http"

	"github.com/Strob0t/CodeForge/internal/domain/routing"
)

// HandleListRoutingStats handles GET /api/v1/routing/stats
func (h *Handlers) HandleListRoutingStats(w http.ResponseWriter, r *http.Request) {
	taskType := r.URL.Query().Get("task_type")
	tier := r.URL.Query().Get("tier")

	stats, err := h.Routing.GetStats(r.Context(), taskType, tier)
	if err != nil {
		writeInternalError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, stats)
}

// HandleRefreshRoutingStats handles POST /api/v1/routing/stats/refresh
func (h *Handlers) HandleRefreshRoutingStats(w http.ResponseWriter, r *http.Request) {
	if err := h.Routing.RefreshStats(r.Context()); err != nil {
		writeInternalError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// HandleListRoutingOutcomes handles GET /api/v1/routing/outcomes
func (h *Handlers) HandleListRoutingOutcomes(w http.ResponseWriter, r *http.Request) {
	limit := queryParamIntClamped(r, "limit", 50, 500)

	outcomes, err := h.Routing.ListOutcomes(r.Context(), limit)
	if err != nil {
		writeInternalError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, outcomes)
}

// HandleCreateRoutingOutcome handles POST /api/v1/routing/outcomes
func (h *Handlers) HandleCreateRoutingOutcome(w http.ResponseWriter, r *http.Request) {
	o, ok := readJSON[routing.RoutingOutcome](w, r, 1<<20)
	if !ok {
		return
	}

	if err := h.Routing.RecordOutcome(r.Context(), &o); err != nil {
		writeInternalError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, o)
}

// HandleSeedFromBenchmarks handles POST /api/v1/routing/seed-from-benchmarks
func (h *Handlers) HandleSeedFromBenchmarks(w http.ResponseWriter, r *http.Request) {
	count, err := h.Routing.SeedFromBenchmarks(r.Context())
	if err != nil {
		writeInternalError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":           "ok",
		"outcomes_created": count,
	})
}
