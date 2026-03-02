package http

import (
	"encoding/json"
	"net/http"
	"strconv"

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
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}

	outcomes, err := h.Routing.ListOutcomes(r.Context(), limit)
	if err != nil {
		writeInternalError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, outcomes)
}

// HandleCreateRoutingOutcome handles POST /api/v1/routing/outcomes
func (h *Handlers) HandleCreateRoutingOutcome(w http.ResponseWriter, r *http.Request) {
	var o routing.RoutingOutcome
	if err := json.NewDecoder(r.Body).Decode(&o); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
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
