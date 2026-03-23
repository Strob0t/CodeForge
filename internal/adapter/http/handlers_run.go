package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/port/eventstore"
	"github.com/Strob0t/CodeForge/internal/service"
)

// RunHandlers groups HTTP handlers for run lifecycle (start, cancel, get)
// and run event retrieval.
type RunHandlers struct {
	Runtime *service.RuntimeService
	Events  eventstore.Store
	Limits  *config.Limits
}

// StartRun handles POST /api/v1/runs
func (rh *RunHandlers) StartRun(w http.ResponseWriter, r *http.Request) {
	req, ok := readJSON[run.StartRequest](w, r, rh.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	if req.TaskID == "" {
		writeError(w, http.StatusBadRequest, "task_id is required")
		return
	}
	if req.AgentID == "" {
		writeError(w, http.StatusBadRequest, "agent_id is required")
		return
	}

	result, err := rh.Runtime.StartRun(r.Context(), &req)
	if err != nil {
		writeDomainError(w, err, "start run failed")
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

// GetRun handles GET /api/v1/runs/{id}
func (rh *RunHandlers) GetRun(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	result, err := rh.Runtime.GetRun(r.Context(), id)
	if err != nil {
		writeDomainError(w, err, "run not found")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// CancelRun handles POST /api/v1/runs/{id}/cancel
func (rh *RunHandlers) CancelRun(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := rh.Runtime.CancelRun(r.Context(), id); err != nil {
		writeDomainError(w, err, "run not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

// ListTaskRuns handles GET /api/v1/tasks/{id}/runs
func (rh *RunHandlers) ListTaskRuns(w http.ResponseWriter, r *http.Request) {
	taskID := chi.URLParam(r, "id")
	runs, err := rh.Runtime.ListRunsByTask(r.Context(), taskID)
	if err != nil {
		writeDomainError(w, err, "task not found")
		return
	}
	if runs == nil {
		runs = []run.Run{}
	}
	writeJSON(w, http.StatusOK, runs)
}

// ListRunEvents handles GET /api/v1/runs/{id}/events
func (rh *RunHandlers) ListRunEvents(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "id")
	if rh.Events == nil {
		writeError(w, http.StatusInternalServerError, "event store not configured")
		return
	}
	events, err := rh.Events.LoadByRun(r.Context(), runID)
	if err != nil {
		writeDomainError(w, err, "run not found")
		return
	}
	if events == nil {
		events = []event.AgentEvent{}
	}
	writeJSON(w, http.StatusOK, events)
}
