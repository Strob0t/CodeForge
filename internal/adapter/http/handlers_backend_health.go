package http

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/Strob0t/CodeForge/internal/service"
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
		writeInternalError(w, fmt.Errorf("backend health check: %w", err))
		return
	}

	type backendsHealthResponse struct {
		Backends []service.BackendHealthEntry `json:"backends"`
	}
	writeJSON(w, http.StatusOK, backendsHealthResponse{
		Backends: backends,
	})
}
