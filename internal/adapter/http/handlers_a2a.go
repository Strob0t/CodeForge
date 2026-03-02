package http

import (
	"net/http"

	"github.com/Strob0t/CodeForge/internal/port/database"
)

type registerRemoteAgentRequest struct {
	Name       string `json:"name"`
	URL        string `json:"url"`
	TrustLevel string `json:"trust_level"`
}

type sendA2ATaskRequest struct {
	SkillID string `json:"skill_id"`
	Prompt  string `json:"prompt"`
}

// RegisterRemoteAgent handles POST /api/v1/a2a/agents
func (h *Handlers) RegisterRemoteAgent(w http.ResponseWriter, r *http.Request) {
	body, ok := readJSON[registerRemoteAgentRequest](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	if !requireField(w, body.Name, "name") || !requireField(w, body.URL, "url") {
		return
	}

	ra, err := h.A2A.RegisterRemoteAgent(r.Context(), body.Name, body.URL, body.TrustLevel)
	if err != nil {
		writeDomainError(w, err, "failed to register remote agent")
		return
	}
	writeJSON(w, http.StatusCreated, ra)
}

// ListRemoteAgents handles GET /api/v1/a2a/agents
func (h *Handlers) ListRemoteAgents(w http.ResponseWriter, r *http.Request) {
	agents, err := h.A2A.ListRemoteAgents(r.Context(), "")
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, agents)
}

// DeleteRemoteAgent handles DELETE /api/v1/a2a/agents/{id}
func (h *Handlers) DeleteRemoteAgent(w http.ResponseWriter, r *http.Request) {
	id := urlParam(r, "id")
	if err := h.A2A.DeleteRemoteAgent(r.Context(), id); err != nil {
		writeDomainError(w, err, "remote agent not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DiscoverRemoteAgent handles POST /api/v1/a2a/agents/{id}/discover
func (h *Handlers) DiscoverRemoteAgent(w http.ResponseWriter, r *http.Request) {
	id := urlParam(r, "id")
	refreshed, err := h.A2A.RefreshAgent(r.Context(), id)
	if err != nil {
		writeDomainError(w, err, "remote agent not found")
		return
	}
	writeJSON(w, http.StatusOK, refreshed)
}

// SendA2ATask handles POST /api/v1/a2a/agents/{id}/send
func (h *Handlers) SendA2ATask(w http.ResponseWriter, r *http.Request) {
	agentID := urlParam(r, "id")
	body, ok := readJSON[sendA2ATaskRequest](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	if !requireField(w, body.Prompt, "prompt") {
		return
	}

	dt, err := h.A2A.SendTask(r.Context(), agentID, body.SkillID, body.Prompt)
	if err != nil {
		writeDomainError(w, err, "failed to send task")
		return
	}
	writeJSON(w, http.StatusCreated, dt)
}

// ListA2ATasks handles GET /api/v1/a2a/tasks
func (h *Handlers) ListA2ATasks(w http.ResponseWriter, r *http.Request) {
	filter := &database.A2ATaskFilter{
		State:     r.URL.Query().Get("state"),
		Direction: r.URL.Query().Get("direction"),
	}
	tasks, _, err := h.A2A.ListTasks(r.Context(), filter)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, tasks)
}

// GetA2ATask handles GET /api/v1/a2a/tasks/{id}
func (h *Handlers) GetA2ATask(w http.ResponseWriter, r *http.Request) {
	id := urlParam(r, "id")
	dt, err := h.A2A.GetTask(r.Context(), id)
	if err != nil {
		writeDomainError(w, err, "task not found")
		return
	}
	writeJSON(w, http.StatusOK, dt)
}

// CancelA2ATask handles POST /api/v1/a2a/tasks/{id}/cancel
func (h *Handlers) CancelA2ATask(w http.ResponseWriter, r *http.Request) {
	id := urlParam(r, "id")
	if err := h.A2A.CancelTask(r.Context(), id); err != nil {
		writeDomainError(w, err, "task not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "canceled"})
}
