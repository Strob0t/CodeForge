package http

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/task"
	"github.com/Strob0t/CodeForge/internal/port/agentbackend"
	"github.com/Strob0t/CodeForge/internal/port/gitprovider"
	"github.com/Strob0t/CodeForge/internal/service"
)

// Handlers holds the HTTP handler dependencies.
type Handlers struct {
	Projects *service.ProjectService
	Tasks    *service.TaskService
}

// ListProjects handles GET /api/v1/projects
func (h *Handlers) ListProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := h.Projects.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if projects == nil {
		projects = []project.Project{}
	}
	writeJSON(w, http.StatusOK, projects)
}

// GetProject handles GET /api/v1/projects/{id}
func (h *Handlers) GetProject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	p, err := h.Projects.Get(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// CreateProject handles POST /api/v1/projects
func (h *Handlers) CreateProject(w http.ResponseWriter, r *http.Request) {
	var req project.CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	p, err := h.Projects.Create(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

// DeleteProject handles DELETE /api/v1/projects/{id}
func (h *Handlers) DeleteProject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.Projects.Delete(r.Context(), id); err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListTasks handles GET /api/v1/projects/{id}/tasks
func (h *Handlers) ListTasks(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	tasks, err := h.Tasks.List(r.Context(), projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if tasks == nil {
		tasks = []task.Task{}
	}
	writeJSON(w, http.StatusOK, tasks)
}

// CreateTask handles POST /api/v1/projects/{id}/tasks
func (h *Handlers) CreateTask(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	var req task.CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.ProjectID = projectID

	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}

	t, err := h.Tasks.Create(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

// GetTask handles GET /api/v1/tasks/{id}
func (h *Handlers) GetTask(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	t, err := h.Tasks.Get(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// ListGitProviders handles GET /api/v1/providers/git
func (h *Handlers) ListGitProviders(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string][]string{
		"providers": gitprovider.Available(),
	})
}

// ListAgentBackends handles GET /api/v1/providers/agent
func (h *Handlers) ListAgentBackends(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string][]string{
		"backends": agentbackend.Available(),
	})
}

// --- Helpers ---

type errorResponse struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error("failed to write JSON response", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}
