package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/Strob0t/CodeForge/internal/domain/conversation"
	"github.com/Strob0t/CodeForge/internal/middleware"
)

func (h *Handlers) CreateConversation(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	req, ok := readJSON[conversation.CreateRequest](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	req.ProjectID = projectID
	conv, err := h.Conversations.Create(r.Context(), req)
	if err != nil {
		writeDomainError(w, err, "create conversation")
		return
	}
	writeJSON(w, http.StatusCreated, conv)
}

// ListConversations handles GET /api/v1/projects/{id}/conversations
// Supports ?limit=N&offset=N query params (default limit=100, offset=0).
func (h *Handlers) ListConversations(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	conversations, err := h.Conversations.ListByProject(r.Context(), projectID)
	if err != nil {
		writeDomainError(w, err, "project not found")
		return
	}
	limit, offset := parsePagination(r, 100)
	conversations = applyPagination(conversations, limit, offset)
	writeJSONList(w, http.StatusOK, conversations)
}

// GetConversation handles GET /api/v1/conversations/{id}
func (h *Handlers) GetConversation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	conv, err := h.Conversations.Get(r.Context(), id)
	if err != nil {
		writeDomainError(w, err, "get conversation")
		return
	}
	writeJSON(w, http.StatusOK, conv)
}

// DeleteConversation handles DELETE /api/v1/conversations/{id}
func (h *Handlers) DeleteConversation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.Conversations.Delete(r.Context(), id); err != nil {
		writeDomainError(w, err, "delete conversation")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListConversationMessages handles GET /api/v1/conversations/{id}/messages
func (h *Handlers) ListConversationMessages(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	messages, err := h.Conversations.ListMessages(r.Context(), id)
	if err != nil {
		writeDomainError(w, err, "conversation not found")
		return
	}
	writeJSONList(w, http.StatusOK, messages)
}

// SendConversationMessage handles POST /api/v1/conversations/{id}/messages.
// When agentic mode is active (via request body or project default), the message
// is dispatched to the Python worker for autonomous tool-using execution.
// Otherwise it falls back to a simple single-turn LLM call.
func (h *Handlers) SendConversationMessage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	req, ok := readJSON[conversation.SendMessageRequest](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}

	// Inject authenticated user ID for per-user key resolution.
	if u := middleware.UserFromContext(r.Context()); u != nil {
		req.UserID = u.ID
	}

	// Route to agentic or simple path — both dispatch via NATS and return 202.
	var err error
	if h.Conversations.IsAgentic(r.Context(), id, &req) {
		err = h.Conversations.SendMessageAgentic(r.Context(), id, &req)
	} else {
		_, err = h.Conversations.SendMessage(r.Context(), id, &req)
	}
	if err != nil {
		writeDomainError(w, err, "send message")
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]string{
		"status":  "dispatched",
		"run_id":  id,
		"message": "Run dispatched. Results will stream via WebSocket.",
	})
}

// StopConversation handles POST /api/v1/conversations/{id}/stop.
// Cancels an active agentic conversation run.
func (h *Handlers) StopConversation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.Conversations.StopConversation(r.Context(), id); err != nil {
		writeDomainError(w, err, "stop conversation")
		return
	}
	// Mark the conversation run as cancelled in RuntimeService so that
	// in-flight NATS tool-call requests are rejected immediately instead
	// of blocking the queue until timeout.
	if h.Runtime != nil {
		h.Runtime.MarkConversationRunCancelled(id)
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled", "conversation_id": id})
}

// --- HITL Approval ---

// ApproveToolCall handles POST /api/v1/runs/{id}/approve/{callId}.
// The user sends a decision ("allow" or "deny") to approve or reject a pending
// tool call that the policy evaluated as "ask".
func (h *Handlers) ApproveToolCall(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "id")
	callID := chi.URLParam(r, "callId")

	type approvalRequest struct {
		Decision string `json:"decision"` // "allow" or "deny"
	}

	req, ok := readJSON[approvalRequest](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	if req.Decision != "allow" && req.Decision != "deny" {
		writeError(w, http.StatusBadRequest, "decision must be 'allow' or 'deny'")
		return
	}

	resolved := h.Runtime.ResolveApproval(runID, callID, req.Decision)
	if !resolved {
		writeError(w, http.StatusNotFound, "no pending approval for this run/call")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status":   "resolved",
		"run_id":   runID,
		"call_id":  callID,
		"decision": req.Decision,
	})
}

// BypassConversationApprovals handles POST /api/v1/conversations/{id}/bypass-approvals.
func (h *Handlers) BypassConversationApprovals(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if h.Runtime != nil {
		h.Runtime.BypassConversationApprovals(id)
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "bypassed", "conversation_id": id})
}

// CompactConversation handles POST /api/v1/conversations/{id}/compact.
// Dispatches a compaction request to summarise the conversation history.
func (h *Handlers) CompactConversation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.Conversations.CompactConversation(r.Context(), id); err != nil {
		writeDomainError(w, err, "compact conversation")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"status":          "compacting",
		"conversation_id": id,
	})
}

// ClearConversation handles POST /api/v1/conversations/{id}/clear.
// Deletes all messages from the conversation.
func (h *Handlers) ClearConversation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.Conversations.ClearConversation(r.Context(), id); err != nil {
		writeDomainError(w, err, "clear conversation")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"status":          "cleared",
		"conversation_id": id,
	})
}

// SetConversationMode handles POST /api/v1/conversations/{id}/mode.
// Sets the agent mode for a conversation.
func (h *Handlers) SetConversationMode(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	type modeRequest struct {
		Mode string `json:"mode"`
	}

	req, ok := readJSON[modeRequest](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	if req.Mode == "" {
		writeError(w, http.StatusBadRequest, "mode is required")
		return
	}

	if err := h.Conversations.SetMode(r.Context(), id, req.Mode); err != nil {
		writeDomainError(w, err, "set conversation mode")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"status":          "mode_changed",
		"conversation_id": id,
		"mode":            req.Mode,
	})
}

// SetConversationModel handles POST /api/v1/conversations/{id}/model.
// Sets a model override for a conversation.
func (h *Handlers) SetConversationModel(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	type modelRequest struct {
		Model string `json:"model"`
	}

	req, ok := readJSON[modelRequest](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	if req.Model == "" {
		writeError(w, http.StatusBadRequest, "model is required")
		return
	}

	if err := h.Conversations.SetModel(r.Context(), id, req.Model); err != nil {
		writeDomainError(w, err, "set conversation model")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"status":          "model_changed",
		"conversation_id": id,
		"model":           req.Model,
	})
}

// RevertToolCall reverts a file edit to its pre-change state.
// POST /api/v1/runs/{id}/revert/{callId}
func (h *Handlers) RevertToolCall(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "id")
	callID := chi.URLParam(r, "callId")

	if h.Checkpoint == nil {
		writeError(w, http.StatusInternalServerError, "checkpoint service not available")
		return
	}

	err := h.Checkpoint.Revert(runID, callID)
	if err != nil {
		writeDomainError(w, err, "revert tool call failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "reverted",
		"run_id":  runID,
		"call_id": callID,
	})
}

// GetAgentConfig handles GET /api/v1/agent-config.
// Returns agent configuration values that the frontend needs (e.g., max_context_tokens).
func (h *Handlers) GetAgentConfig(w http.ResponseWriter, _ *http.Request) {
	maxContextTokens := 128_000 // default
	if h.AgentConfig != nil {
		maxContextTokens = h.AgentConfig.MaxContextTokens
	}

	writeJSON(w, http.StatusOK, map[string]int{
		"max_context_tokens": maxContextTokens,
	})
}
