package http

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Strob0t/CodeForge/internal/adapter/litellm"
	"github.com/Strob0t/CodeForge/internal/domain/experience"
	lspDomain "github.com/Strob0t/CodeForge/internal/domain/lsp"
	"github.com/Strob0t/CodeForge/internal/domain/memory"
	"github.com/Strob0t/CodeForge/internal/domain/microagent"
	"github.com/Strob0t/CodeForge/internal/domain/skill"
)

// --- Dev Tools ---

// BenchmarkPrompt handles POST /api/v1/dev/benchmark
// Sends a prompt to LiteLLM and returns the response with timing/token metrics.
// Guarded by the DEV_MODE environment variable.
func (h *Handlers) BenchmarkPrompt(w http.ResponseWriter, r *http.Request) {
	if strings.ToLower(os.Getenv("DEV_MODE")) != "true" {
		writeError(w, http.StatusForbidden, "dev mode not enabled")
		return
	}

	type benchmarkRequest struct {
		Model        string  `json:"model"`
		Prompt       string  `json:"prompt"`
		SystemPrompt string  `json:"system_prompt"`
		Temperature  float64 `json:"temperature"`
		MaxTokens    int     `json:"max_tokens"`
	}

	req, ok := readJSON[benchmarkRequest](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	if req.Model == "" {
		writeError(w, http.StatusBadRequest, "model is required")
		return
	}
	if req.Prompt == "" {
		writeError(w, http.StatusBadRequest, "prompt is required")
		return
	}

	messages := []litellm.ChatMessage{}
	if req.SystemPrompt != "" {
		messages = append(messages, litellm.ChatMessage{Role: "system", Content: req.SystemPrompt})
	}
	messages = append(messages, litellm.ChatMessage{Role: "user", Content: req.Prompt})

	start := time.Now()
	resp, err := h.LiteLLM.ChatCompletion(r.Context(), litellm.ChatCompletionRequest{
		Model:       req.Model,
		Messages:    messages,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
	})
	latencyMs := time.Since(start).Milliseconds()

	if err != nil {
		slog.Error("benchmark prompt failed", "error", err)
		writeError(w, http.StatusBadGateway, "LLM call failed: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"content":    resp.Content,
		"model":      resp.Model,
		"tokens_in":  resp.TokensIn,
		"tokens_out": resp.TokensOut,
		"latency_ms": latencyMs,
	})
}

// --- LSP (Language Server Protocol) ---

// StartLSP handles POST /api/v1/projects/{id}/lsp/start
func (h *Handlers) StartLSP(w http.ResponseWriter, r *http.Request) {
	if h.LSP == nil {
		writeError(w, http.StatusServiceUnavailable, "LSP integration is not enabled")
		return
	}
	projectID := chi.URLParam(r, "id")
	proj, err := h.Projects.Get(r.Context(), projectID)
	if err != nil {
		writeDomainError(w, err, "project not found")
		return
	}
	if proj.WorkspacePath == "" {
		writeError(w, http.StatusBadRequest, "project has no workspace; clone or adopt first")
		return
	}

	var body struct {
		Languages []string `json:"languages"`
	}
	// Body is optional — auto-detect if empty.
	_ = json.NewDecoder(r.Body).Decode(&body)

	if err := h.LSP.StartServers(r.Context(), projectID, proj.WorkspacePath, body.Languages); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "started"})
}

// StopLSP handles POST /api/v1/projects/{id}/lsp/stop
func (h *Handlers) StopLSP(w http.ResponseWriter, r *http.Request) {
	if h.LSP == nil {
		writeError(w, http.StatusServiceUnavailable, "LSP integration is not enabled")
		return
	}
	projectID := chi.URLParam(r, "id")
	if err := h.LSP.StopServers(r.Context(), projectID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

// LSPStatus handles GET /api/v1/projects/{id}/lsp/status
func (h *Handlers) LSPStatus(w http.ResponseWriter, r *http.Request) {
	if h.LSP == nil {
		writeJSON(w, http.StatusOK, []lspDomain.ServerInfo{})
		return
	}
	projectID := chi.URLParam(r, "id")
	infos := h.LSP.Status(projectID)
	if infos == nil {
		infos = []lspDomain.ServerInfo{}
	}
	writeJSON(w, http.StatusOK, infos)
}

// LSPDiagnostics handles GET /api/v1/projects/{id}/lsp/diagnostics
func (h *Handlers) LSPDiagnostics(w http.ResponseWriter, r *http.Request) {
	if h.LSP == nil {
		writeJSON(w, http.StatusOK, []lspDomain.Diagnostic{})
		return
	}
	projectID := chi.URLParam(r, "id")
	uri := r.URL.Query().Get("uri")
	diags := h.LSP.Diagnostics(projectID, uri)
	if diags == nil {
		diags = []lspDomain.Diagnostic{}
	}
	writeJSON(w, http.StatusOK, diags)
}

// lspPositionRequest is the shared request body for definition/references/hover.
type lspPositionRequest struct {
	URI       string `json:"uri"`
	Line      int    `json:"line"`
	Character int    `json:"character"`
}

// LSPDefinition handles POST /api/v1/projects/{id}/lsp/definition
func (h *Handlers) LSPDefinition(w http.ResponseWriter, r *http.Request) {
	if h.LSP == nil {
		writeError(w, http.StatusServiceUnavailable, "LSP integration is not enabled")
		return
	}
	projectID := chi.URLParam(r, "id")
	req, ok := readJSON[lspPositionRequest](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	if req.URI == "" {
		writeError(w, http.StatusBadRequest, "uri is required")
		return
	}
	locs, err := h.LSP.Definition(r.Context(), projectID, req.URI, lspDomain.Position{
		Line: req.Line, Character: req.Character,
	})
	if err != nil {
		writeDomainError(w, err, "definition lookup failed")
		return
	}
	if locs == nil {
		locs = []lspDomain.Location{}
	}
	writeJSON(w, http.StatusOK, locs)
}

// LSPReferences handles POST /api/v1/projects/{id}/lsp/references
func (h *Handlers) LSPReferences(w http.ResponseWriter, r *http.Request) {
	if h.LSP == nil {
		writeError(w, http.StatusServiceUnavailable, "LSP integration is not enabled")
		return
	}
	projectID := chi.URLParam(r, "id")
	req, ok := readJSON[lspPositionRequest](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	if req.URI == "" {
		writeError(w, http.StatusBadRequest, "uri is required")
		return
	}
	locs, err := h.LSP.References(r.Context(), projectID, req.URI, lspDomain.Position{
		Line: req.Line, Character: req.Character,
	})
	if err != nil {
		writeDomainError(w, err, "references lookup failed")
		return
	}
	if locs == nil {
		locs = []lspDomain.Location{}
	}
	writeJSON(w, http.StatusOK, locs)
}

// LSPDocumentSymbols handles POST /api/v1/projects/{id}/lsp/symbols
func (h *Handlers) LSPDocumentSymbols(w http.ResponseWriter, r *http.Request) {
	if h.LSP == nil {
		writeError(w, http.StatusServiceUnavailable, "LSP integration is not enabled")
		return
	}
	projectID := chi.URLParam(r, "id")
	var req struct {
		URI string `json:"uri"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, h.Limits.MaxRequestBodySize)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.URI == "" {
		writeError(w, http.StatusBadRequest, "uri is required")
		return
	}
	symbols, err := h.LSP.DocumentSymbols(r.Context(), projectID, req.URI)
	if err != nil {
		writeDomainError(w, err, "symbol lookup failed")
		return
	}
	if symbols == nil {
		symbols = []lspDomain.DocumentSymbol{}
	}
	writeJSON(w, http.StatusOK, symbols)
}

// LSPHover handles POST /api/v1/projects/{id}/lsp/hover
func (h *Handlers) LSPHover(w http.ResponseWriter, r *http.Request) {
	if h.LSP == nil {
		writeError(w, http.StatusServiceUnavailable, "LSP integration is not enabled")
		return
	}
	projectID := chi.URLParam(r, "id")
	req, ok := readJSON[lspPositionRequest](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	if req.URI == "" {
		writeError(w, http.StatusBadRequest, "uri is required")
		return
	}
	result, err := h.LSP.Hover(r.Context(), projectID, req.URI, lspDomain.Position{
		Line: req.Line, Character: req.Character,
	})
	if err != nil {
		writeDomainError(w, err, "hover lookup failed")
		return
	}
	if result == nil {
		writeJSON(w, http.StatusOK, nil)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// --- Memory Handlers (Phase 22B) ---

// ListMemories handles GET /api/v1/projects/{id}/memories.
func (h *Handlers) ListMemories(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	mems, err := h.Memory.ListByProject(r.Context(), projectID)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	if mems == nil {
		mems = []memory.Memory{}
	}
	writeJSON(w, http.StatusOK, mems)
}

// StoreMemory handles POST /api/v1/projects/{id}/memories.
func (h *Handlers) StoreMemory(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	req, ok := readJSON[memory.CreateRequest](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	req.ProjectID = projectID
	if err := h.Memory.Store(r.Context(), &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "dispatched"})
}

// RecallMemories handles POST /api/v1/projects/{id}/memories/recall.
func (h *Handlers) RecallMemories(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	req, ok := readJSON[memory.RecallRequest](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	req.ProjectID = projectID
	if err := h.Memory.Recall(r.Context(), req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "dispatched"})
}

// --- Experience Pool Handlers (Phase 22B) ---

// ListExperienceEntries handles GET /api/v1/projects/{id}/experience.
func (h *Handlers) ListExperienceEntries(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	entries, err := h.ExperiencePool.ListByProject(r.Context(), projectID)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	if entries == nil {
		entries = []experience.Entry{}
	}
	writeJSON(w, http.StatusOK, entries)
}

// DeleteExperienceEntry handles DELETE /api/v1/experience/{id}.
func (h *Handlers) DeleteExperienceEntry(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.ExperiencePool.Delete(r.Context(), id); err != nil {
		writeInternalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Microagent Handlers (Phase 22C) ---

// ListMicroagents handles GET /api/v1/projects/{id}/microagents.
func (h *Handlers) ListMicroagents(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	mas, err := h.Microagents.List(r.Context(), projectID)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	if mas == nil {
		mas = []microagent.Microagent{}
	}
	writeJSON(w, http.StatusOK, mas)
}

// CreateMicroagent handles POST /api/v1/projects/{id}/microagents.
func (h *Handlers) CreateMicroagent(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	req, ok := readJSON[microagent.CreateRequest](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	req.ProjectID = projectID
	m, err := h.Microagents.Create(r.Context(), &req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, m)
}

// GetMicroagent handles GET /api/v1/microagents/{id}.
func (h *Handlers) GetMicroagent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	m, err := h.Microagents.Get(r.Context(), id)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

// UpdateMicroagent handles PUT /api/v1/microagents/{id}.
func (h *Handlers) UpdateMicroagent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	req, ok := readJSON[microagent.UpdateRequest](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	m, err := h.Microagents.Update(r.Context(), id, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, m)
}

// DeleteMicroagent handles DELETE /api/v1/microagents/{id}.
func (h *Handlers) DeleteMicroagent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.Microagents.Delete(r.Context(), id); err != nil {
		writeInternalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Skill Handlers (Phase 22D) ---

// ListSkills handles GET /api/v1/projects/{id}/skills.
func (h *Handlers) ListSkills(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	sk, err := h.Skills.List(r.Context(), projectID)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	if sk == nil {
		sk = []skill.Skill{}
	}
	writeJSON(w, http.StatusOK, sk)
}

// CreateSkill handles POST /api/v1/projects/{id}/skills.
func (h *Handlers) CreateSkill(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	req, ok := readJSON[skill.CreateRequest](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	req.ProjectID = projectID
	s, err := h.Skills.Create(r.Context(), &req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, s)
}

// GetSkill handles GET /api/v1/skills/{id}.
func (h *Handlers) GetSkill(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	s, err := h.Skills.Get(r.Context(), id)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s)
}

// UpdateSkill handles PUT /api/v1/skills/{id}.
func (h *Handlers) UpdateSkill(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	req, ok := readJSON[skill.UpdateRequest](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	s, err := h.Skills.Update(r.Context(), id, &req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s)
}

// DeleteSkill handles DELETE /api/v1/skills/{id}.
func (h *Handlers) DeleteSkill(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.Skills.Delete(r.Context(), id); err != nil {
		writeInternalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// HandleFeedbackCallback handles POST /api/v1/feedback/{run_id}/{call_id}.
// This is the callback endpoint for email/Slack approval links.
func (h *Handlers) HandleFeedbackCallback(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "run_id")
	callID := chi.URLParam(r, "call_id")
	decision := r.URL.Query().Get("decision")

	if decision != "allow" && decision != "deny" {
		writeError(w, http.StatusBadRequest, "decision must be 'allow' or 'deny'")
		return
	}

	resolved := h.Runtime.ResolveApproval(runID, callID, decision)
	if !resolved {
		writeError(w, http.StatusNotFound, "no pending approval for this run/call")
		return
	}

	// Log audit entry via RuntimeService.
	_ = h.Runtime.LogFeedbackAudit(r.Context(), runID, callID, "", "web_callback", decision, "")

	writeJSON(w, http.StatusOK, map[string]string{
		"status":   "resolved",
		"decision": decision,
	})
}

// ListFeedbackAudit handles GET /api/v1/runs/{id}/feedback.
func (h *Handlers) ListFeedbackAudit(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "id")
	entries, err := h.Runtime.ListFeedbackAudit(r.Context(), runID)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, entries)
}
