package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/Strob0t/CodeForge/internal/domain/knowledgebase"
)

// --- Knowledge Base Handlers ---

// ListKnowledgeBases handles GET /api/v1/knowledge-bases
func (h *Handlers) ListKnowledgeBases(w http.ResponseWriter, r *http.Request) {
	kbs, err := h.KnowledgeBases.List(r.Context())
	if err != nil {
		writeDomainError(w, err, "failed to list knowledge bases")
		return
	}
	if kbs == nil {
		kbs = []knowledgebase.KnowledgeBase{}
	}
	writeJSON(w, http.StatusOK, kbs)
}

// GetKnowledgeBase handles GET /api/v1/knowledge-bases/{id}
func (h *Handlers) GetKnowledgeBase(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	kb, err := h.KnowledgeBases.Get(r.Context(), id)
	if err != nil {
		writeDomainError(w, err, "knowledge base not found")
		return
	}
	writeJSON(w, http.StatusOK, kb)
}

// CreateKnowledgeBase handles POST /api/v1/knowledge-bases
func (h *Handlers) CreateKnowledgeBase(w http.ResponseWriter, r *http.Request) {
	req, ok := readJSON[knowledgebase.CreateRequest](w, r)
	if !ok {
		return
	}

	kb, err := h.KnowledgeBases.Create(r.Context(), &req)
	if err != nil {
		writeDomainError(w, err, "failed to create knowledge base")
		return
	}
	writeJSON(w, http.StatusCreated, kb)
}

// UpdateKnowledgeBase handles PUT /api/v1/knowledge-bases/{id}
func (h *Handlers) UpdateKnowledgeBase(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	req, ok := readJSON[knowledgebase.UpdateRequest](w, r)
	if !ok {
		return
	}

	kb, err := h.KnowledgeBases.Update(r.Context(), id, req)
	if err != nil {
		writeDomainError(w, err, "failed to update knowledge base")
		return
	}
	writeJSON(w, http.StatusOK, kb)
}

// DeleteKnowledgeBase handles DELETE /api/v1/knowledge-bases/{id}
func (h *Handlers) DeleteKnowledgeBase(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.KnowledgeBases.Delete(r.Context(), id); err != nil {
		writeDomainError(w, err, "failed to delete knowledge base")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// IndexKnowledgeBase handles POST /api/v1/knowledge-bases/{id}/index
func (h *Handlers) IndexKnowledgeBase(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.KnowledgeBases.RequestIndex(r.Context(), id); err != nil {
		writeDomainError(w, err, "failed to index knowledge base")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "indexing"})
}

// AttachKnowledgeBaseToScope handles POST /api/v1/scopes/{id}/knowledge-bases
func (h *Handlers) AttachKnowledgeBaseToScope(w http.ResponseWriter, r *http.Request) {
	scopeID := chi.URLParam(r, "id")

	req, ok := readJSON[struct {
		KnowledgeBaseID string `json:"knowledge_base_id"`
	}](w, r)
	if !ok {
		return
	}
	if req.KnowledgeBaseID == "" {
		writeError(w, http.StatusBadRequest, "knowledge_base_id is required")
		return
	}

	if err := h.KnowledgeBases.AttachToScope(r.Context(), scopeID, req.KnowledgeBaseID); err != nil {
		writeDomainError(w, err, "failed to attach knowledge base to scope")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DetachKnowledgeBaseFromScope handles DELETE /api/v1/scopes/{id}/knowledge-bases/{kbid}
func (h *Handlers) DetachKnowledgeBaseFromScope(w http.ResponseWriter, r *http.Request) {
	scopeID := chi.URLParam(r, "id")
	kbID := chi.URLParam(r, "kbid")

	if err := h.KnowledgeBases.DetachFromScope(r.Context(), scopeID, kbID); err != nil {
		writeDomainError(w, err, "failed to detach knowledge base from scope")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListScopeKnowledgeBases handles GET /api/v1/scopes/{id}/knowledge-bases
func (h *Handlers) ListScopeKnowledgeBases(w http.ResponseWriter, r *http.Request) {
	scopeID := chi.URLParam(r, "id")

	kbs, err := h.KnowledgeBases.ListByScope(r.Context(), scopeID)
	if err != nil {
		writeDomainError(w, err, "failed to list scope knowledge bases")
		return
	}
	if kbs == nil {
		kbs = []knowledgebase.KnowledgeBase{}
	}
	writeJSON(w, http.StatusOK, kbs)
}
