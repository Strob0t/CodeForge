package http

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// --- RepoMap Endpoints ---

// GetRepoMap handles GET /api/v1/projects/{id}/repomap
func (h *Handlers) GetRepoMap(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	m, err := h.RepoMap.Get(r.Context(), projectID)
	if err != nil {
		writeDomainError(w, err, "repo map not found")
		return
	}
	writeJSON(w, http.StatusOK, m)
}

// GenerateRepoMap handles POST /api/v1/projects/{id}/repomap
func (h *Handlers) GenerateRepoMap(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	var req struct {
		ActiveFiles []string `json:"active_files"`
	}
	// Body is optional; empty body is fine.
	r.Body = http.MaxBytesReader(w, r.Body, h.Limits.MaxRequestBodySize)
	_ = json.NewDecoder(r.Body).Decode(&req)

	if err := h.RepoMap.RequestGeneration(r.Context(), projectID, req.ActiveFiles); err != nil {
		writeDomainError(w, err, "project not found")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "generating"})
}

// --- Retrieval Endpoints ---

// IndexProject handles POST /api/v1/projects/{id}/index
func (h *Handlers) IndexProject(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	var req struct {
		EmbeddingModel string `json:"embedding_model"`
	}
	// Body is optional; empty body is fine.
	r.Body = http.MaxBytesReader(w, r.Body, h.Limits.MaxRequestBodySize)
	_ = json.NewDecoder(r.Body).Decode(&req)

	if err := h.Retrieval.RequestIndex(r.Context(), projectID, "", req.EmbeddingModel); err != nil {
		writeDomainError(w, err, "project not found")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "building"})
}

// GetIndexStatus handles GET /api/v1/projects/{id}/index
func (h *Handlers) GetIndexStatus(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	info := h.Retrieval.GetIndexStatus(projectID)
	if info == nil {
		writeError(w, http.StatusNotFound, "no index found for project")
		return
	}
	writeJSON(w, http.StatusOK, info)
}

// SearchProject handles POST /api/v1/projects/{id}/search
func (h *Handlers) SearchProject(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	req, ok := readJSON[struct {
		Query          string  `json:"query"`
		TopK           int     `json:"top_k"`
		BM25Weight     float64 `json:"bm25_weight"`
		SemanticWeight float64 `json:"semantic_weight"`
	}](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	if req.Query == "" {
		writeError(w, http.StatusBadRequest, "query is required")
		return
	}
	if len(req.Query) > h.Limits.MaxQueryLength {
		writeError(w, http.StatusBadRequest, "query exceeds maximum length of 2000 characters")
		return
	}

	// Clamp top_k to safe bounds.
	topK := req.TopK
	if topK <= 0 {
		topK = 20
	} else if topK > 500 {
		topK = 500
	}

	result, err := h.Retrieval.SearchSync(r.Context(), projectID, req.Query, topK, req.BM25Weight, req.SemanticWeight)
	if err != nil {
		slog.Error("search timed out", "error", err)
		writeError(w, http.StatusGatewayTimeout, "search timed out")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// --- Retrieval Sub-Agent Endpoints (Phase 6C) ---

// AgentSearchProject handles POST /api/v1/projects/{id}/search/agent
func (h *Handlers) AgentSearchProject(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	req, ok := readJSON[struct {
		Query      string `json:"query"`
		TopK       int    `json:"top_k"`
		MaxQueries int    `json:"max_queries"`
		Model      string `json:"model"`
		Rerank     *bool  `json:"rerank"` // pointer to distinguish absent (use config default) from explicit false
	}](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	if req.Query == "" {
		writeError(w, http.StatusBadRequest, "query is required")
		return
	}
	if len(req.Query) > h.Limits.MaxQueryLength {
		writeError(w, http.StatusBadRequest, "query exceeds maximum length of 2000 characters")
		return
	}

	// Apply defaults from config, clamp to safe bounds.
	defaultModel, defaultMaxQueries, defaultRerank := h.Retrieval.SubAgentDefaults()
	topK := req.TopK
	if topK <= 0 {
		topK = 20
	} else if topK > 500 {
		topK = 500
	}
	maxQueries := req.MaxQueries
	if maxQueries <= 0 {
		maxQueries = defaultMaxQueries
	} else if maxQueries > 20 {
		maxQueries = 20
	}
	model := req.Model
	if model == "" {
		model = defaultModel
	}
	rerank := defaultRerank
	if req.Rerank != nil {
		rerank = *req.Rerank
	}

	// Look up project-specific expansion prompt from config.
	var expansionPrompt string
	if proj, projErr := h.Projects.Get(r.Context(), projectID); projErr == nil && proj.Config != nil {
		expansionPrompt = proj.Config["expansion_prompt"]
	}

	result, err := h.Retrieval.SubAgentSearchSync(r.Context(), projectID, req.Query, topK, maxQueries, model, rerank, expansionPrompt)
	if err != nil {
		slog.Error("search timed out", "error", err)
		writeError(w, http.StatusGatewayTimeout, "search timed out")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// --- GraphRAG Endpoints (Phase 6D) ---

// BuildGraph handles POST /api/v1/projects/{id}/graph/build
func (h *Handlers) BuildGraph(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	proj, err := h.Projects.Get(r.Context(), projectID)
	if err != nil {
		writeDomainError(w, err, "project not found")
		return
	}

	if err := h.Graph.RequestBuild(r.Context(), projectID, proj.WorkspacePath); err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "building"})
}

// GetGraphStatus handles GET /api/v1/projects/{id}/graph/status
func (h *Handlers) GetGraphStatus(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	info := h.Graph.GetStatus(projectID)
	if info == nil {
		writeError(w, http.StatusNotFound, "no graph found for project")
		return
	}
	writeJSON(w, http.StatusOK, info)
}

// SearchGraph handles POST /api/v1/projects/{id}/graph/search
func (h *Handlers) SearchGraph(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	req, ok := readJSON[struct {
		SeedSymbols []string `json:"seed_symbols"`
		MaxHops     int      `json:"max_hops"`
		TopK        int      `json:"top_k"`
	}](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
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

	result, err := h.Graph.SearchSync(r.Context(), projectID, req.SeedSymbols, maxHops, topK)
	if err != nil {
		slog.Error("search timed out", "error", err)
		writeError(w, http.StatusGatewayTimeout, "search timed out")
		return
	}
	writeJSON(w, http.StatusOK, result)
}
