package http

import (
	"net/http"
	"strconv"
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
