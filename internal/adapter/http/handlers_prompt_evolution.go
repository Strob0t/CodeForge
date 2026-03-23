package http

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// ListPromptEvolutionVariants handles GET /api/v1/prompt-evolution/variants
func (h *Handlers) ListPromptEvolutionVariants(w http.ResponseWriter, r *http.Request) {
	if h.PromptEvolution == nil {
		writeError(w, http.StatusServiceUnavailable, "prompt evolution not enabled")
		return
	}

	modeID := r.URL.Query().Get("mode_id")
	status := r.URL.Query().Get("status")

	variants, err := h.PromptEvolution.ListVariants(r.Context(), modeID, status)
	if err != nil {
		writeInternalError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, variants)
}

// GetPromptEvolutionStatus handles GET /api/v1/prompt-evolution/status
func (h *Handlers) GetPromptEvolutionStatus(w http.ResponseWriter, r *http.Request) {
	if h.PromptEvolution == nil {
		writeError(w, http.StatusServiceUnavailable, "prompt evolution not enabled")
		return
	}

	status := h.PromptEvolution.GetStatus()
	writeJSON(w, http.StatusOK, status)
}

// RevertPromptEvolutionMode handles POST /api/v1/prompt-evolution/revert/{modeId}
func (h *Handlers) RevertPromptEvolutionMode(w http.ResponseWriter, r *http.Request) {
	if h.PromptEvolution == nil {
		writeError(w, http.StatusServiceUnavailable, "prompt evolution not enabled")
		return
	}

	modeID := chi.URLParam(r, "modeId")
	if modeID == "" {
		writeError(w, http.StatusBadRequest, "mode_id is required")
		return
	}

	tenantID := tenantctx.FromContext(r.Context())
	if err := h.PromptEvolution.RevertMode(r.Context(), tenantID, modeID); err != nil {
		writeInternalError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "reverted", "mode_id": modeID})
}

// PromotePromptEvolutionVariant handles POST /api/v1/prompt-evolution/promote/{variantId}
func (h *Handlers) PromotePromptEvolutionVariant(w http.ResponseWriter, r *http.Request) {
	if h.PromptEvolution == nil {
		writeError(w, http.StatusServiceUnavailable, "prompt evolution not enabled")
		return
	}

	variantID := chi.URLParam(r, "variantId")
	if variantID == "" {
		writeError(w, http.StatusBadRequest, "variant_id is required")
		return
	}

	tenantID := tenantctx.FromContext(r.Context())
	if err := h.PromptEvolution.PromoteVariant(r.Context(), tenantID, variantID); err != nil {
		writeDomainError(w, err, "variant not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "promoted", "variant_id": variantID})
}

// triggerReflectRequest is the request body for POST /api/v1/prompt-evolution/reflect.
type triggerReflectRequest struct {
	ModeID        string                       `json:"mode_id"`
	ModelFamily   string                       `json:"model_family"`
	CurrentPrompt string                       `json:"current_prompt"`
	Failures      []map[string]json.RawMessage `json:"failures"`
}

// TriggerPromptEvolutionReflect handles POST /api/v1/prompt-evolution/reflect
func (h *Handlers) TriggerPromptEvolutionReflect(w http.ResponseWriter, r *http.Request) {
	if h.PromptEvolution == nil {
		writeError(w, http.StatusServiceUnavailable, "prompt evolution not enabled")
		return
	}

	var req triggerReflectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.ModeID == "" {
		writeError(w, http.StatusBadRequest, "mode_id is required")
		return
	}
	if req.ModelFamily == "" {
		writeError(w, http.StatusBadRequest, "model_family is required")
		return
	}
	if req.CurrentPrompt == "" {
		writeError(w, http.StatusBadRequest, "current_prompt is required")
		return
	}

	tenantID := tenantctx.FromContext(r.Context())
	if err := h.PromptEvolution.TriggerReflection(r.Context(), tenantID, req.ModeID, req.ModelFamily, req.CurrentPrompt, req.Failures); err != nil {
		writeInternalError(w, err)
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]string{"status": "reflection_triggered"})
}
