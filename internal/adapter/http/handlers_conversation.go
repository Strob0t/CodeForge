package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/Strob0t/CodeForge/internal/domain/conversation"
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
func (h *Handlers) ListConversations(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	conversations, err := h.Conversations.ListByProject(r.Context(), projectID)
	if err != nil {
		writeDomainError(w, err, "project not found")
		return
	}
	if conversations == nil {
		conversations = []conversation.Conversation{}
	}
	writeJSON(w, http.StatusOK, conversations)
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
	if messages == nil {
		messages = []conversation.Message{}
	}
	writeJSON(w, http.StatusOK, messages)
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

	// Route to agentic path when applicable.
	if h.Conversations.IsAgentic(r.Context(), id, req) {
		if err := h.Conversations.SendMessageAgentic(r.Context(), id, req); err != nil {
			writeDomainError(w, err, "send agentic message")
			return
		}
		// Agentic mode returns immediately; results stream via WebSocket.
		writeJSON(w, http.StatusAccepted, map[string]string{
			"status":  "dispatched",
			"run_id":  id,
			"message": "Agentic run dispatched. Results will stream via WebSocket.",
		})
		return
	}

	msg, err := h.Conversations.SendMessage(r.Context(), id, req)
	if err != nil {
		writeDomainError(w, err, "send message")
		return
	}
	writeJSON(w, http.StatusCreated, msg)
}

// StopConversation handles POST /api/v1/conversations/{id}/stop.
// Cancels an active agentic conversation run.
func (h *Handlers) StopConversation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.Conversations.StopConversation(r.Context(), id); err != nil {
		writeDomainError(w, err, "stop conversation")
		return
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
