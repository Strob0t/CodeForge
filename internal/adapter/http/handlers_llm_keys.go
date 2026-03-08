package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/Strob0t/CodeForge/internal/domain/llmkey"
	"github.com/Strob0t/CodeForge/internal/middleware"
)

// ListLLMKeys handles GET /api/v1/llm-keys
func (h *Handlers) ListLLMKeys(w http.ResponseWriter, r *http.Request) {
	u := middleware.UserFromContext(r.Context())
	if u == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	keys, err := h.LLMKeys.List(r.Context(), u.ID)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSONList(w, http.StatusOK, keys)
}

// CreateLLMKey handles POST /api/v1/llm-keys
func (h *Handlers) CreateLLMKey(w http.ResponseWriter, r *http.Request) {
	u := middleware.UserFromContext(r.Context())
	if u == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	req, ok := readJSON[llmkey.CreateRequest](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}

	key, err := h.LLMKeys.Create(r.Context(), u.ID, req)
	if err != nil {
		writeDomainError(w, err, "create llm key failed")
		return
	}
	writeJSON(w, http.StatusCreated, key)
}

// DeleteLLMKey handles DELETE /api/v1/llm-keys/{id}
func (h *Handlers) DeleteLLMKey(w http.ResponseWriter, r *http.Request) {
	u := middleware.UserFromContext(r.Context())
	if u == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	id := chi.URLParam(r, "id")
	if err := h.LLMKeys.Delete(r.Context(), id, u.ID); err != nil {
		writeDomainError(w, err, "llm key not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
