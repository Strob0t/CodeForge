package http

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	bp "github.com/Strob0t/CodeForge/internal/domain/branchprotection"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/run"
)

// --- Branch Protection Rules ---

// ListBranchProtectionRules handles GET /api/v1/projects/{id}/branch-rules
func (h *Handlers) ListBranchProtectionRules(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	rules, err := h.BranchProtection.ListRules(r.Context(), projectID)
	if err != nil {
		writeDomainError(w, err, "project not found")
		return
	}
	if rules == nil {
		rules = []bp.ProtectionRule{}
	}
	writeJSON(w, http.StatusOK, rules)
}

// CreateBranchProtectionRule handles POST /api/v1/projects/{id}/branch-rules
func (h *Handlers) CreateBranchProtectionRule(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	req, ok := readJSON[bp.CreateRuleRequest](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	req.ProjectID = projectID

	rule, err := h.BranchProtection.CreateRule(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, rule)
}

// GetBranchProtectionRule handles GET /api/v1/branch-rules/{id}
func (h *Handlers) GetBranchProtectionRule(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	rule, err := h.BranchProtection.GetRule(r.Context(), id)
	if err != nil {
		writeDomainError(w, err, "branch protection rule not found")
		return
	}
	writeJSON(w, http.StatusOK, rule)
}

// UpdateBranchProtectionRule handles PUT /api/v1/branch-rules/{id}
func (h *Handlers) UpdateBranchProtectionRule(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	req, ok := readJSON[bp.UpdateRuleRequest](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}

	rule, err := h.BranchProtection.UpdateRule(r.Context(), id, req)
	if err != nil {
		writeDomainError(w, err, "branch protection rule not found")
		return
	}
	writeJSON(w, http.StatusOK, rule)
}

// DeleteBranchProtectionRule handles DELETE /api/v1/branch-rules/{id}
func (h *Handlers) DeleteBranchProtectionRule(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.BranchProtection.DeleteRule(r.Context(), id); err != nil {
		writeDomainError(w, err, "branch protection rule not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// --- Replay / Audit Trail ---

// ListRunCheckpoints handles GET /api/v1/runs/{id}/checkpoints
func (h *Handlers) ListRunCheckpoints(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	checkpoints, err := h.Replay.ListCheckpoints(r.Context(), id)
	if err != nil {
		writeDomainError(w, err, "run not found")
		return
	}
	if checkpoints == nil {
		checkpoints = []event.AgentEvent{}
	}
	writeJSON(w, http.StatusOK, checkpoints)
}

// ReplayRun handles POST /api/v1/runs/{id}/replay
func (h *Handlers) ReplayRun(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	req, ok := readJSON[event.ReplayRequest](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	req.RunID = id

	result, err := h.Replay.Replay(r.Context(), req)
	if err != nil {
		writeDomainError(w, err, "run not found")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// GlobalAuditTrail handles GET /api/v1/audit
func (h *Handlers) GlobalAuditTrail(w http.ResponseWriter, r *http.Request) {
	filter := event.AuditFilter{
		Action: r.URL.Query().Get("action"),
	}
	cursor := r.URL.Query().Get("cursor")
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	page, err := h.Replay.AuditTrail(r.Context(), &filter, cursor, limit)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

// ProjectAuditTrail handles GET /api/v1/projects/{id}/audit
func (h *Handlers) ProjectAuditTrail(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	filter := event.AuditFilter{
		ProjectID: projectID,
		Action:    r.URL.Query().Get("action"),
	}
	cursor := r.URL.Query().Get("cursor")
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	page, err := h.Replay.AuditTrail(r.Context(), &filter, cursor, limit)
	if err != nil {
		writeDomainError(w, err, "project not found")
		return
	}
	writeJSON(w, http.StatusOK, page)
}

// --- Sessions ---

// ResumeRun handles POST /api/v1/runs/{id}/resume
func (h *Handlers) ResumeRun(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req run.ResumeRequest
	r.Body = http.MaxBytesReader(w, r.Body, h.Limits.MaxRequestBodySize)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req = run.ResumeRequest{} // empty body is OK
	}
	req.RunID = id

	sess, err := h.Sessions.Resume(r.Context(), req)
	if err != nil {
		writeDomainError(w, err, "run not found")
		return
	}
	writeJSON(w, http.StatusCreated, sess)
}

// ForkRun handles POST /api/v1/runs/{id}/fork
func (h *Handlers) ForkRun(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req run.ForkRequest
	r.Body = http.MaxBytesReader(w, r.Body, h.Limits.MaxRequestBodySize)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req = run.ForkRequest{} // empty body is OK
	}
	req.RunID = id

	sess, err := h.Sessions.Fork(r.Context(), req)
	if err != nil {
		writeDomainError(w, err, "run not found")
		return
	}
	writeJSON(w, http.StatusCreated, sess)
}

// RewindRun handles POST /api/v1/runs/{id}/rewind
func (h *Handlers) RewindRun(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req run.RewindRequest
	r.Body = http.MaxBytesReader(w, r.Body, h.Limits.MaxRequestBodySize)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req = run.RewindRequest{} // empty body is OK
	}
	req.RunID = id

	sess, err := h.Sessions.Rewind(r.Context(), req)
	if err != nil {
		writeDomainError(w, err, "run not found")
		return
	}
	writeJSON(w, http.StatusCreated, sess)
}

// ListProjectSessions handles GET /api/v1/projects/{id}/sessions
func (h *Handlers) ListProjectSessions(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	sessions, err := h.Sessions.ListSessions(r.Context(), projectID)
	if err != nil {
		writeDomainError(w, err, "project not found")
		return
	}
	if sessions == nil {
		sessions = []run.Session{}
	}
	writeJSON(w, http.StatusOK, sessions)
}

// GetSession handles GET /api/v1/sessions/{id}
func (h *Handlers) GetSession(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sess, err := h.Sessions.GetSession(r.Context(), id)
	if err != nil {
		writeDomainError(w, err, "session not found")
		return
	}
	writeJSON(w, http.StatusOK, sess)
}
