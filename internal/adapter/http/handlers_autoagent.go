package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// StartAutoAgent handles POST /api/v1/projects/{id}/auto-agent/start
func (h *Handlers) StartAutoAgent(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	if !requireField(w, projectID, "project id") {
		return
	}

	aa, err := h.AutoAgent.Start(r.Context(), projectID)
	if err != nil {
		writeDomainError(w, err, "start auto-agent")
		return
	}
	writeJSON(w, http.StatusOK, aa)
}

// StopAutoAgent handles POST /api/v1/projects/{id}/auto-agent/stop
func (h *Handlers) StopAutoAgent(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	if !requireField(w, projectID, "project id") {
		return
	}

	if err := h.AutoAgent.Stop(r.Context(), projectID); err != nil {
		writeDomainError(w, err, "stop auto-agent")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopping"})
}

// GetAutoAgentStatus handles GET /api/v1/projects/{id}/auto-agent/status
func (h *Handlers) GetAutoAgentStatus(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	if !requireField(w, projectID, "project id") {
		return
	}

	aa, err := h.AutoAgent.Status(r.Context(), projectID)
	if err != nil {
		writeDomainError(w, err, "get auto-agent status")
		return
	}
	writeJSON(w, http.StatusOK, aa)
}
