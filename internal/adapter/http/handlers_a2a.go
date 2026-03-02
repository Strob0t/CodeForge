package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

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

// --- Push Notification Config Handlers (Phase 27O) ---

type createPushConfigRequest struct {
	URL   string `json:"url"`
	Token string `json:"token"`
}

// CreateA2APushConfig handles POST /api/v1/a2a/tasks/{id}/push-config
func (h *Handlers) CreateA2APushConfig(w http.ResponseWriter, r *http.Request) {
	taskID := urlParam(r, "id")
	body, ok := readJSON[createPushConfigRequest](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	if !requireField(w, body.URL, "url") {
		return
	}

	id, err := h.A2A.CreatePushConfig(r.Context(), taskID, body.URL, body.Token)
	if err != nil {
		writeDomainError(w, err, "failed to create push config")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

// ListA2APushConfigs handles GET /api/v1/a2a/tasks/{id}/push-config
func (h *Handlers) ListA2APushConfigs(w http.ResponseWriter, r *http.Request) {
	taskID := urlParam(r, "id")
	configs, err := h.A2A.ListPushConfigs(r.Context(), taskID)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	if configs == nil {
		configs = []database.A2APushConfig{}
	}
	writeJSON(w, http.StatusOK, configs)
}

// DeleteA2APushConfig handles DELETE /api/v1/a2a/push-config/{id}
func (h *Handlers) DeleteA2APushConfig(w http.ResponseWriter, r *http.Request) {
	id := urlParam(r, "id")
	if err := h.A2A.DeletePushConfig(r.Context(), id); err != nil {
		writeDomainError(w, err, "push config not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// SubscribeA2ATask handles GET /api/v1/a2a/tasks/{id}/subscribe (SSE streaming)
func (h *Handlers) SubscribeA2ATask(w http.ResponseWriter, r *http.Request) {
	taskID := urlParam(r, "id")

	// Verify task exists.
	task, err := h.A2A.GetTask(r.Context(), taskID)
	if err != nil {
		writeDomainError(w, err, "task not found")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// Send initial state.
	writeSSEEvent(w, flusher, "status", map[string]string{
		"task_id": task.ID,
		"state":   string(task.State),
	})

	// If already in terminal state, close immediately.
	if isTerminalA2AState(string(task.State)) {
		writeSSEEvent(w, flusher, "done", map[string]string{"task_id": task.ID})
		return
	}

	// Poll for state changes until terminal or client disconnect.
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	lastState := task.State
	ctx := r.Context()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			current, err := h.A2A.GetTask(ctx, taskID)
			if err != nil {
				writeSSEEvent(w, flusher, "error", map[string]string{"error": "task lookup failed"})
				return
			}
			if current.State != lastState {
				lastState = current.State
				writeSSEEvent(w, flusher, "status", map[string]string{
					"task_id": current.ID,
					"state":   string(current.State),
				})
			}
			if isTerminalA2AState(string(current.State)) {
				writeSSEEvent(w, flusher, "done", map[string]string{"task_id": current.ID})
				return
			}
		}
	}
}

// writeSSEEvent writes a single SSE event to the response.
func writeSSEEvent(w http.ResponseWriter, flusher http.Flusher, event string, data any) {
	payload, _ := json.Marshal(data)
	_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, payload)
	flusher.Flush()
}

// isTerminalA2AState returns true for A2A states that will not change further.
func isTerminalA2AState(s string) bool {
	switch s {
	case "completed", "failed", "canceled", "rejected":
		return true
	}
	return false
}
