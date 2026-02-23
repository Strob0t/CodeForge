package http

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	cfcontext "github.com/Strob0t/CodeForge/internal/domain/context"
)

// --- Retrieval Scope Handlers ---

// CreateScope handles POST /api/v1/scopes
func (h *Handlers) CreateScope(w http.ResponseWriter, r *http.Request) {
	var req cfcontext.CreateScopeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	sc, err := h.Scope.Create(r.Context(), req)
	if err != nil {
		writeDomainError(w, err, "failed to create scope")
		return
	}
	writeJSON(w, http.StatusCreated, sc)
}

// ListScopes handles GET /api/v1/scopes
func (h *Handlers) ListScopes(w http.ResponseWriter, r *http.Request) {
	scopes, err := h.Scope.List(r.Context())
	if err != nil {
		writeDomainError(w, err, "failed to list scopes")
		return
	}
	if scopes == nil {
		scopes = []cfcontext.RetrievalScope{}
	}
	writeJSON(w, http.StatusOK, scopes)
}

// GetScope handles GET /api/v1/scopes/{id}
func (h *Handlers) GetScope(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sc, err := h.Scope.Get(r.Context(), id)
	if err != nil {
		writeDomainError(w, err, "scope not found")
		return
	}
	writeJSON(w, http.StatusOK, sc)
}

// UpdateScope handles PUT /api/v1/scopes/{id}
func (h *Handlers) UpdateScope(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req cfcontext.UpdateScopeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	sc, err := h.Scope.Update(r.Context(), id, req)
	if err != nil {
		writeDomainError(w, err, "failed to update scope")
		return
	}
	writeJSON(w, http.StatusOK, sc)
}

// DeleteScope handles DELETE /api/v1/scopes/{id}
func (h *Handlers) DeleteScope(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.Scope.Delete(r.Context(), id); err != nil {
		writeDomainError(w, err, "failed to delete scope")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// AddProjectToScope handles POST /api/v1/scopes/{id}/projects
func (h *Handlers) AddProjectToScope(w http.ResponseWriter, r *http.Request) {
	scopeID := chi.URLParam(r, "id")

	var req struct {
		ProjectID string `json:"project_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.ProjectID == "" {
		writeError(w, http.StatusBadRequest, "project_id is required")
		return
	}

	if err := h.Scope.AddProject(r.Context(), scopeID, req.ProjectID); err != nil {
		writeDomainError(w, err, "failed to add project to scope")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RemoveProjectFromScope handles DELETE /api/v1/scopes/{id}/projects/{pid}
func (h *Handlers) RemoveProjectFromScope(w http.ResponseWriter, r *http.Request) {
	scopeID := chi.URLParam(r, "id")
	projectID := chi.URLParam(r, "pid")

	if err := h.Scope.RemoveProject(r.Context(), scopeID, projectID); err != nil {
		writeDomainError(w, err, "failed to remove project from scope")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// SearchScope handles POST /api/v1/scopes/{id}/search
func (h *Handlers) SearchScope(w http.ResponseWriter, r *http.Request) {
	scopeID := chi.URLParam(r, "id")

	var req struct {
		Query          string  `json:"query"`
		TopK           int     `json:"top_k"`
		BM25Weight     float64 `json:"bm25_weight"`
		SemanticWeight float64 `json:"semantic_weight"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Query == "" {
		writeError(w, http.StatusBadRequest, "query is required")
		return
	}
	if len(req.Query) > maxQueryLength {
		writeError(w, http.StatusBadRequest, "query exceeds maximum length of 2000 characters")
		return
	}

	topK := req.TopK
	if topK <= 0 {
		topK = 20
	} else if topK > 500 {
		topK = 500
	}

	hits, err := h.Scope.SearchScope(r.Context(), scopeID, req.Query, topK, req.BM25Weight, req.SemanticWeight)
	if err != nil {
		writeDomainError(w, err, "scope search failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"results": hits,
		"count":   len(hits),
	})
}

// SearchScopeGraph handles POST /api/v1/scopes/{id}/graph/search
func (h *Handlers) SearchScopeGraph(w http.ResponseWriter, r *http.Request) {
	scopeID := chi.URLParam(r, "id")

	var req struct {
		SeedSymbols []string `json:"seed_symbols"`
		MaxHops     int      `json:"max_hops"`
		TopK        int      `json:"top_k"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.SeedSymbols) == 0 {
		writeError(w, http.StatusBadRequest, "seed_symbols is required")
		return
	}

	maxHops := req.MaxHops
	if maxHops <= 0 {
		maxHops = 2
	} else if maxHops > 10 {
		maxHops = 10
	}
	topK := req.TopK
	if topK <= 0 {
		topK = 10
	} else if topK > 500 {
		topK = 500
	}

	hits, err := h.Scope.SearchScopeGraph(r.Context(), scopeID, req.SeedSymbols, maxHops, topK)
	if err != nil {
		writeDomainError(w, err, "scope graph search failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"results": hits,
		"count":   len(hits),
	})
}
