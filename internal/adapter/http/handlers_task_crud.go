package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/task"
	"github.com/Strob0t/CodeForge/internal/service"
)

// TaskHandlers groups HTTP handlers for task CRUD, claim,
// active work, and active agents.
type TaskHandlers struct {
	Tasks      *service.TaskService
	ActiveWork *service.ActiveWorkService
	Limits     *config.Limits
}

// ListTasks handles GET /api/v1/projects/{id}/tasks
func (th *TaskHandlers) ListTasks(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	tasks, err := th.Tasks.List(r.Context(), projectID)
	if err != nil {
		writeDomainError(w, err, "project not found")
		return
	}
	if tasks == nil {
		tasks = []task.Task{}
	}
	writeJSON(w, http.StatusOK, tasks)
}

// CreateTask handles POST /api/v1/projects/{id}/tasks
func (th *TaskHandlers) CreateTask(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	req, ok := readJSON[task.CreateRequest](w, r, th.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	req.ProjectID = projectID

	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}

	t, err := th.Tasks.Create(r.Context(), req)
	if err != nil {
		writeDomainError(w, err, "task creation failed")
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

// GetTask handles GET /api/v1/tasks/{id}
func (th *TaskHandlers) GetTask(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	t, err := th.Tasks.Get(r.Context(), id)
	if err != nil {
		writeDomainError(w, err, "task not found")
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// ListActiveWork handles GET /api/v1/projects/{id}/active-work
func (th *TaskHandlers) ListActiveWork(w http.ResponseWriter, r *http.Request) {
	if th.ActiveWork == nil {
		writeJSON(w, http.StatusOK, []task.ActiveWorkItem{})
		return
	}
	projectID := chi.URLParam(r, "id")
	items, err := th.ActiveWork.ListActiveWork(r.Context(), projectID)
	if err != nil {
		writeDomainError(w, err, "project not found")
		return
	}
	if items == nil {
		items = []task.ActiveWorkItem{}
	}
	writeJSON(w, http.StatusOK, items)
}

// ClaimTask handles POST /api/v1/tasks/{id}/claim
func (th *TaskHandlers) ClaimTask(w http.ResponseWriter, r *http.Request) {
	taskID := chi.URLParam(r, "id")

	b, ok := readJSON[struct {
		AgentID string `json:"agent_id"`
	}](w, r, th.Limits.MaxRequestBodySize)
	if !ok {
		return
	}

	if !requireField(w, b.AgentID, "agent_id") {
		return
	}

	result, err := th.ActiveWork.ClaimTask(r.Context(), taskID, b.AgentID)
	if err != nil {
		writeDomainError(w, err, "task not found")
		return
	}
	if !result.Claimed {
		writeJSON(w, http.StatusConflict, result)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
