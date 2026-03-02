package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/Strob0t/CodeForge/internal/domain/goal"
)

// ListProjectGoals handles GET /api/v1/projects/{id}/goals.
func (h *Handlers) ListProjectGoals(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	goals, err := h.GoalDiscovery.List(r.Context(), projectID)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	if goals == nil {
		goals = []goal.ProjectGoal{}
	}
	writeJSON(w, http.StatusOK, goals)
}

// CreateProjectGoal handles POST /api/v1/projects/{id}/goals.
func (h *Handlers) CreateProjectGoal(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	req, ok := readJSON[goal.CreateRequest](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	g, err := h.GoalDiscovery.Create(r.Context(), projectID, &req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, g)
}

// DetectProjectGoals handles POST /api/v1/projects/{id}/goals/detect.
func (h *Handlers) DetectProjectGoals(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	proj, err := h.Projects.Get(r.Context(), projectID)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	if proj.WorkspacePath == "" {
		writeError(w, http.StatusBadRequest, "project has no workspace path")
		return
	}

	result, err := h.GoalDiscovery.DetectAndImport(r.Context(), projectID, proj.WorkspacePath)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// GetProjectGoal handles GET /api/v1/goals/{id}.
func (h *Handlers) GetProjectGoal(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	g, err := h.GoalDiscovery.Get(r.Context(), id)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, g)
}

// UpdateProjectGoal handles PUT /api/v1/goals/{id}.
func (h *Handlers) UpdateProjectGoal(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	req, ok := readJSON[goal.UpdateRequest](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	g, err := h.GoalDiscovery.Update(r.Context(), id, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, g)
}

// DeleteProjectGoal handles DELETE /api/v1/goals/{id}.
func (h *Handlers) DeleteProjectGoal(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.GoalDiscovery.Delete(r.Context(), id); err != nil {
		writeInternalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
