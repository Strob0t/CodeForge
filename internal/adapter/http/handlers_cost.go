package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// --- Cost Endpoints ---

// GlobalCostSummary handles GET /api/v1/costs
func (h *Handlers) GlobalCostSummary(w http.ResponseWriter, r *http.Request) {
	summaries, err := h.Cost.GlobalSummary(r.Context())
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSONList(w, http.StatusOK, summaries)
}

// ProjectCostSummary handles GET /api/v1/projects/{id}/costs
func (h *Handlers) ProjectCostSummary(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	summary, err := h.Cost.ProjectSummary(r.Context(), projectID)
	if err != nil {
		writeDomainError(w, err, "project not found")
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

// ProjectCostByModel handles GET /api/v1/projects/{id}/costs/by-model
func (h *Handlers) ProjectCostByModel(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	models, err := h.Cost.ByModel(r.Context(), projectID)
	if err != nil {
		writeDomainError(w, err, "project not found")
		return
	}
	writeJSONList(w, http.StatusOK, models)
}

// ProjectCostTimeSeries handles GET /api/v1/projects/{id}/costs/daily
func (h *Handlers) ProjectCostTimeSeries(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	days := queryParamInt(r, "days", 30)
	series, err := h.Cost.TimeSeries(r.Context(), projectID, days)
	if err != nil {
		writeDomainError(w, err, "project not found")
		return
	}
	writeJSONList(w, http.StatusOK, series)
}

// ProjectRecentRuns handles GET /api/v1/projects/{id}/costs/runs
func (h *Handlers) ProjectRecentRuns(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	limit := queryParamInt(r, "limit", 20)
	runs, err := h.Cost.RecentRuns(r.Context(), projectID, limit)
	if err != nil {
		writeDomainError(w, err, "project not found")
		return
	}
	writeJSONList(w, http.StatusOK, runs)
}

// ProjectCostByTool handles GET /api/v1/projects/{id}/costs/by-tool
func (h *Handlers) ProjectCostByTool(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	tools, err := h.Cost.ByTool(r.Context(), projectID)
	if err != nil {
		writeDomainError(w, err, "project not found")
		return
	}
	writeJSONList(w, http.StatusOK, tools)
}

// RunCostByTool handles GET /api/v1/runs/{id}/costs/by-tool
func (h *Handlers) RunCostByTool(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "id")
	tools, err := h.Cost.ByToolForRun(r.Context(), runID)
	if err != nil {
		writeDomainError(w, err, "run not found")
		return
	}
	writeJSONList(w, http.StatusOK, tools)
}
