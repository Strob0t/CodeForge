package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/Strob0t/CodeForge/internal/domain/dashboard"
)

// DashboardStats handles GET /api/v1/dashboard/stats
func (h *Handlers) DashboardStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.Dashboard.Stats(r.Context())
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

// ProjectHealth handles GET /api/v1/projects/{id}/health
func (h *Handlers) ProjectHealth(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	health, err := h.Dashboard.ProjectHealth(r.Context(), projectID)
	if err != nil {
		writeDomainError(w, err, "project not found")
		return
	}
	writeJSON(w, http.StatusOK, health)
}

// DashboardRunOutcomes handles GET /api/v1/dashboard/charts/run-outcomes
func (h *Handlers) DashboardRunOutcomes(w http.ResponseWriter, r *http.Request) {
	days := queryParamIntClamped(r, "days", 7, 500)
	outcomes, err := h.Dashboard.RunOutcomes(r.Context(), days)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	if outcomes == nil {
		outcomes = []dashboard.RunOutcome{}
	}
	writeJSON(w, http.StatusOK, outcomes)
}

// DashboardAgentPerformance handles GET /api/v1/dashboard/charts/agent-performance
func (h *Handlers) DashboardAgentPerformance(w http.ResponseWriter, r *http.Request) {
	agents, err := h.Dashboard.AgentPerformance(r.Context())
	if err != nil {
		writeInternalError(w, err)
		return
	}
	if agents == nil {
		agents = []dashboard.AgentPerf{}
	}
	writeJSON(w, http.StatusOK, agents)
}

// DashboardModelUsage handles GET /api/v1/dashboard/charts/model-usage
func (h *Handlers) DashboardModelUsage(w http.ResponseWriter, r *http.Request) {
	models, err := h.Dashboard.ModelUsage(r.Context())
	if err != nil {
		writeInternalError(w, err)
		return
	}
	if models == nil {
		models = []dashboard.ModelUsage{}
	}
	writeJSON(w, http.StatusOK, models)
}

// DashboardCostByProject handles GET /api/v1/dashboard/charts/cost-by-project
func (h *Handlers) DashboardCostByProject(w http.ResponseWriter, r *http.Request) {
	costs, err := h.Dashboard.CostByProject(r.Context())
	if err != nil {
		writeInternalError(w, err)
		return
	}
	if costs == nil {
		costs = []dashboard.ProjectCost{}
	}
	writeJSON(w, http.StatusOK, costs)
}

// DashboardCostTrend handles GET /api/v1/dashboard/charts/cost-trend
func (h *Handlers) DashboardCostTrend(w http.ResponseWriter, r *http.Request) {
	days := queryParamIntClamped(r, "days", 30, 500)
	trend, err := h.Dashboard.CostTrend(r.Context(), days)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, trend)
}
