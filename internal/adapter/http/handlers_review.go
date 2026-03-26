package http

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/Strob0t/CodeForge/internal/domain/boundary"
)

// GetProjectBoundaries handles GET /api/v1/projects/{id}/boundaries
func (h *Handlers) GetProjectBoundaries(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	cfg, err := h.Boundaries.GetBoundaries(r.Context(), projectID)
	if err != nil {
		writeDomainError(w, err, "boundaries not found")
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

// UpdateProjectBoundaries handles PUT /api/v1/projects/{id}/boundaries
func (h *Handlers) UpdateProjectBoundaries(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	cfg, ok := readJSON[boundary.ProjectBoundaryConfig](w, r, 1<<20)
	if !ok {
		return
	}
	cfg.ProjectID = projectID
	if err := h.Boundaries.UpdateBoundaries(r.Context(), &cfg); err != nil {
		writeDomainError(w, err, "failed to update boundaries")
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

// TriggerBoundaryAnalysis handles POST /api/v1/projects/{id}/boundaries/analyze
func (h *Handlers) TriggerBoundaryAnalysis(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	if h.ReviewTrigger == nil {
		writeError(w, http.StatusServiceUnavailable, "review trigger service not configured")
		return
	}
	triggered, err := h.ReviewTrigger.TriggerReview(r.Context(), projectID, "", "manual")
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"triggered": triggered})
}

// TriggerReviewRefactor handles POST /api/v1/projects/{id}/review-refactor
func (h *Handlers) TriggerReviewRefactor(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	var body struct {
		CommitSHA string `json:"commit_sha"`
	}
	// Body is optional — log but do not reject on parse errors.
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		slog.Debug("optional body parse skipped", "handler", "TriggerReviewRefactor", "error", err)
	}

	if h.ReviewTrigger == nil {
		writeError(w, http.StatusServiceUnavailable, "review trigger service not configured")
		return
	}
	triggered, err := h.ReviewTrigger.TriggerReview(r.Context(), projectID, body.CommitSHA, "manual")
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"triggered": triggered})
}

// ApproveRun handles POST /api/v1/runs/{id}/approve
func (h *Handlers) ApproveRun(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "id")
	body, ok := readJSON[struct {
		PlanID string `json:"plan_id"`
		StepID string `json:"step_id"`
	}](w, r, 1<<20)
	if !ok {
		return
	}
	if err := h.Orchestrator.ApproveStep(r.Context(), body.PlanID, body.StepID); err != nil {
		writeDomainError(w, err, "failed to approve run "+runID)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "approved"})
}

// RejectRun handles POST /api/v1/runs/{id}/reject
func (h *Handlers) RejectRun(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "id")
	body, ok := readJSON[struct {
		PlanID string `json:"plan_id"`
		StepID string `json:"step_id"`
	}](w, r, 1<<20)
	if !ok {
		return
	}
	if err := h.Orchestrator.RejectStep(r.Context(), body.PlanID, body.StepID); err != nil {
		writeDomainError(w, err, "failed to reject run "+runID)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "rejected"})
}

// ApproveRunPartial handles POST /api/v1/runs/{id}/approve-partial
func (h *Handlers) ApproveRunPartial(w http.ResponseWriter, r *http.Request) {
	// Partial approval is a future enhancement — for now, delegates to full approval
	h.ApproveRun(w, r)
}
