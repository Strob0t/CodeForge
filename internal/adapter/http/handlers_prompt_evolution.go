package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

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
