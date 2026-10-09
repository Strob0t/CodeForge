package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	cfcontext "github.com/Strob0t/CodeForge/internal/domain/context"
)

// InitSharedContext handles POST /api/v1/teams/{teamId}/shared-context
func (h *Handlers) InitSharedContext(w http.ResponseWriter, r *http.Request) {
	teamID := chi.URLParam(r, "teamId")
	if teamID == "" {
		writeError(w, http.StatusBadRequest, "missing teamId")
		return
	}
	body, ok := readJSON[struct {
		ProjectID string `json:"project_id"`
	}](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	sc, err := h.SharedContext.InitForTeam(r.Context(), teamID, body.ProjectID)
	if err != nil {
		writeDomainError(w, err, "failed to initialize shared context")
		return
	}
	writeJSON(w, http.StatusCreated, sc)
}

// GetSharedContext handles GET /api/v1/teams/{teamId}/shared-context
func (h *Handlers) GetSharedContext(w http.ResponseWriter, r *http.Request) {
	teamID := chi.URLParam(r, "teamId")
	if teamID == "" {
		writeError(w, http.StatusBadRequest, "missing teamId")
		return
	}
	sc, err := h.SharedContext.Get(r.Context(), teamID)
	if err != nil {
		writeDomainError(w, err, "shared context not found")
		return
	}
	writeJSON(w, http.StatusOK, sc)
}

// AddSharedContextItem handles POST /api/v1/teams/{teamId}/shared-context/items
func (h *Handlers) AddSharedContextItem(w http.ResponseWriter, r *http.Request) {
	teamID := chi.URLParam(r, "teamId")
	if teamID == "" {
		writeError(w, http.StatusBadRequest, "missing teamId")
		return
	}
	body, ok := readJSON[cfcontext.AddSharedItemRequest](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	body.TeamID = teamID
	item, err := h.SharedContext.AddItem(r.Context(), body)
	if err != nil {
		writeDomainError(w, err, "failed to add shared context item")
		return
	}
	writeJSON(w, http.StatusCreated, item)
}
