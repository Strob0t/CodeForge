package http

import (
	"context"
	"net/http"
	"time"
)

// CheckBackendHealth handles GET /api/v1/backends/health.
// Dispatches to the Python worker via NATS and returns health status of all backends.
func (h *Handlers) CheckBackendHealth(w http.ResponseWriter, r *http.Request) {
	if h.BackendHealth == nil {
		writeError(w, http.StatusServiceUnavailable, "backend health service not initialized")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	backends, err := h.BackendHealth.CheckHealth(ctx)
	if err != nil {
		writeError(w, http.StatusGatewayTimeout, "backend health check failed: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"backends": backends,
	})
}
