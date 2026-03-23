package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/agent"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/resource"
	"github.com/Strob0t/CodeForge/internal/service"
)

// AgentHandlers groups HTTP handlers for agent CRUD, dispatch,
// inbox, and state management.
type AgentHandlers struct {
	Agents *service.AgentService
	Limits *config.Limits
}

// ListAgents handles GET /api/v1/projects/{id}/agents
// Supports ?limit=N&offset=N query params (default limit=100, offset=0).
func (ah *AgentHandlers) ListAgents(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	agents, err := ah.Agents.List(r.Context(), projectID)
	if err != nil {
		writeDomainError(w, err, "project not found")
		return
	}
	if agents == nil {
		agents = []agent.Agent{}
	}
	limit, offset := parsePagination(r, 100)
	agents = applyPagination(agents, limit, offset)
	writeJSON(w, http.StatusOK, agents)
}

// CreateAgent handles POST /api/v1/projects/{id}/agents
func (ah *AgentHandlers) CreateAgent(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	req, ok := readJSON[struct {
		Name           string            `json:"name"`
		Backend        string            `json:"backend"`
		Config         map[string]string `json:"config"`
		ResourceLimits *resource.Limits  `json:"resource_limits,omitempty"`
	}](w, r, ah.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Backend == "" {
		writeError(w, http.StatusBadRequest, "backend is required")
		return
	}

	a, err := ah.Agents.Create(r.Context(), projectID, req.Name, req.Backend, req.Config, req.ResourceLimits)
	if err != nil {
		writeDomainError(w, err, "create agent failed")
		return
	}
	writeJSON(w, http.StatusCreated, a)
}

// GetAgent handles GET /api/v1/agents/{id}
func (ah *AgentHandlers) GetAgent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	a, err := ah.Agents.Get(r.Context(), id)
	if err != nil {
		writeDomainError(w, err, "agent not found")
		return
	}
	writeJSON(w, http.StatusOK, a)
}

// DeleteAgent handles DELETE /api/v1/agents/{id}
func (ah *AgentHandlers) DeleteAgent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := ah.Agents.Delete(r.Context(), id); err != nil {
		writeDomainError(w, err, "agent not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DispatchTask handles POST /api/v1/agents/{id}/dispatch
func (ah *AgentHandlers) DispatchTask(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "id")

	req, ok := readJSON[struct {
		TaskID string `json:"task_id"`
	}](w, r, ah.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	if req.TaskID == "" {
		writeError(w, http.StatusBadRequest, "task_id is required")
		return
	}

	if err := ah.Agents.Dispatch(r.Context(), agentID, req.TaskID); err != nil {
		writeDomainError(w, err, "agent or task not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "dispatched"})
}

// StopAgentTask handles POST /api/v1/agents/{id}/stop
func (ah *AgentHandlers) StopAgentTask(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "id")

	req, ok := readJSON[struct {
		TaskID string `json:"task_id"`
	}](w, r, ah.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	if req.TaskID == "" {
		writeError(w, http.StatusBadRequest, "task_id is required")
		return
	}

	if err := ah.Agents.StopTask(r.Context(), agentID, req.TaskID); err != nil {
		writeDomainError(w, err, "agent or task not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

// ListAgentInbox handles GET /api/v1/agents/{id}/inbox
func (ah *AgentHandlers) ListAgentInbox(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "id")
	unreadOnly := r.URL.Query().Get("unread") == "true"

	msgs, err := ah.Agents.GetInbox(r.Context(), agentID, unreadOnly)
	if err != nil {
		writeDomainError(w, err, "list inbox failed")
		return
	}
	writeJSON(w, http.StatusOK, msgs)
}

// SendAgentMessage handles POST /api/v1/agents/{id}/inbox
func (ah *AgentHandlers) SendAgentMessage(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "id")

	req, ok := readJSON[struct {
		FromAgent string `json:"from_agent"`
		Content   string `json:"content"`
		Priority  int    `json:"priority"`
	}](w, r, ah.Limits.MaxRequestBodySize)
	if !ok {
		return
	}

	msg := &agent.InboxMessage{
		AgentID:   agentID,
		FromAgent: req.FromAgent,
		Content:   req.Content,
		Priority:  req.Priority,
	}
	if err := ah.Agents.SendMessage(r.Context(), msg); err != nil {
		writeDomainError(w, err, "send message failed")
		return
	}
	writeJSON(w, http.StatusCreated, msg)
}

// MarkInboxRead handles POST /api/v1/agents/{id}/inbox/{msgId}/read
func (ah *AgentHandlers) MarkInboxRead(w http.ResponseWriter, r *http.Request) {
	msgID := chi.URLParam(r, "msgId")
	if err := ah.Agents.MarkRead(r.Context(), msgID); err != nil {
		writeDomainError(w, err, "message not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "read"})
}

// GetAgentState handles GET /api/v1/agents/{id}/state
func (ah *AgentHandlers) GetAgentState(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	a, err := ah.Agents.Get(r.Context(), id)
	if err != nil {
		writeDomainError(w, err, "agent not found")
		return
	}
	writeJSON(w, http.StatusOK, a.State)
}

// UpdateAgentState handles PUT /api/v1/agents/{id}/state
func (ah *AgentHandlers) UpdateAgentState(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	state, ok := readJSON[map[string]string](w, r, ah.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	if err := ah.Agents.UpdateState(r.Context(), id, state); err != nil {
		writeDomainError(w, err, "agent not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// ListTaskEvents handles GET /api/v1/tasks/{id}/events
func (ah *AgentHandlers) ListTaskEvents(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	events, err := ah.Agents.LoadTaskEvents(r.Context(), id)
	if err != nil {
		writeDomainError(w, err, "events not found")
		return
	}
	if events == nil {
		events = []event.AgentEvent{}
	}
	writeJSON(w, http.StatusOK, events)
}

// ListActiveAgents handles GET /api/v1/projects/{id}/agents/active (Phase 23D War Room).
func (ah *AgentHandlers) ListActiveAgents(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	agents, err := ah.Agents.List(r.Context(), projectID)
	if err != nil {
		writeDomainError(w, err, "project not found")
		return
	}

	active := make([]agent.Agent, 0)
	for i := range agents {
		if agents[i].Status == "running" {
			active = append(active, agents[i])
		}
	}
	writeJSON(w, http.StatusOK, active)
}
