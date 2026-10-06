package http

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/boundary"
	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/domain/review"
	"github.com/Strob0t/CodeForge/internal/service"
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

// reviewTriggerResponse answers a review trigger with the plan it started.
type reviewTriggerResponse struct {
	Triggered bool   `json:"triggered"`
	PlanID    string `json:"plan_id,omitempty"`
}

// writeReviewTrigger answers 202 with the started plan, or the reason
// nothing started: 409 when the project already has an active review
// pipeline or the agent belongs to another plan (S6-F 7).
func writeReviewTrigger(w http.ResponseWriter, p *plan.ExecutionPlan, err error) {
	switch {
	case errors.Is(err, service.ErrReviewPipelineUnavailable):
		writeError(w, http.StatusServiceUnavailable, "review pipeline not configured")
	case errors.Is(err, review.ErrPipelineActive):
		writeError(w, http.StatusConflict, review.ErrPipelineActive.Error())
	case errors.Is(err, review.ErrAgentInUse):
		writeError(w, http.StatusConflict, review.ErrAgentInUse.Error())
	case errors.Is(err, domain.ErrConflict):
		writeError(w, http.StatusConflict, "the review pipeline conflicts with another plan of the project")
	case err != nil:
		writeDomainError(w, err, "project not found")
	default:
		writeJSON(w, http.StatusAccepted, reviewTriggerResponse{Triggered: true, PlanID: p.ID})
	}
}

// TriggerBoundaryAnalysis handles POST /api/v1/projects/{id}/boundaries/analyze
func (h *Handlers) TriggerBoundaryAnalysis(w http.ResponseWriter, r *http.Request) {
	if h.ReviewTrigger == nil {
		writeError(w, http.StatusServiceUnavailable, "review pipeline not configured")
		return
	}
	p, err := h.ReviewTrigger.TriggerBoundaryAnalysis(r.Context(), chi.URLParam(r, "id"))
	writeReviewTrigger(w, p, err)
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
		writeError(w, http.StatusServiceUnavailable, "review pipeline not configured")
		return
	}
	p, err := h.ReviewTrigger.TriggerReview(r.Context(), projectID, body.CommitSHA)
	writeReviewTrigger(w, p, err)
}

// reviewDecisionRequest names the plan step whose refactoring (run {id}) is
// decided.
type reviewDecisionRequest struct {
	PlanID string `json:"plan_id"`
	StepID string `json:"step_id"`
}

// decideReview keeps (approve) or undoes (reject) the refactoring of a review
// step. The answer says whether HEAD was moved back and, if not, why
// (service.ReviewDecision).
func (h *Handlers) decideReview(w http.ResponseWriter, r *http.Request, approve bool) {
	if h.ReviewPipeline == nil {
		writeError(w, http.StatusServiceUnavailable, "review pipeline not configured")
		return
	}
	runID := chi.URLParam(r, "id")
	body, ok := readJSON[reviewDecisionRequest](w, r, 1<<20)
	if !ok {
		return
	}
	if !requireField(w, body.PlanID, "plan_id") || !requireField(w, body.StepID, "step_id") {
		return
	}
	decision, err := h.ReviewPipeline.Decide(r.Context(), runID, body.PlanID, body.StepID, approve)
	if err != nil {
		writeDomainError(w, err, "no refactoring of run "+runID+" waits in this step")
		return
	}
	writeJSON(w, http.StatusOK, decision)
}

// ListPendingReviewDecisions handles GET /api/v1/projects/{id}/review/pending:
// the refactorings of the project that wait for a keep or undo decision.
func (h *Handlers) ListPendingReviewDecisions(w http.ResponseWriter, r *http.Request) {
	if h.ReviewPipeline == nil {
		writeError(w, http.StatusServiceUnavailable, "review pipeline not configured")
		return
	}
	pending, err := h.ReviewPipeline.PendingDecisions(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeDomainError(w, err, "project not found")
		return
	}
	writeJSON(w, http.StatusOK, pending)
}

// ApproveRun handles POST /api/v1/runs/{id}/approve
func (h *Handlers) ApproveRun(w http.ResponseWriter, r *http.Request) {
	h.decideReview(w, r, true)
}

// RejectRun handles POST /api/v1/runs/{id}/reject
func (h *Handlers) RejectRun(w http.ResponseWriter, r *http.Request) {
	h.decideReview(w, r, false)
}
