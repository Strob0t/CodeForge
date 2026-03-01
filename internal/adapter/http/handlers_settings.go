package http

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/Strob0t/CodeForge/internal/domain/review"
	"github.com/Strob0t/CodeForge/internal/domain/roadmap"
	"github.com/Strob0t/CodeForge/internal/domain/settings"
	"github.com/Strob0t/CodeForge/internal/domain/tenant"
	"github.com/Strob0t/CodeForge/internal/domain/vcsaccount"
	"github.com/Strob0t/CodeForge/internal/middleware"
)

// --- Tenant Endpoints ---

// ListTenants handles GET /api/v1/tenants
func (h *Handlers) ListTenants(w http.ResponseWriter, r *http.Request) {
	tenants, err := h.Tenants.List(r.Context())
	if err != nil {
		writeInternalError(w, err)
		return
	}
	if tenants == nil {
		tenants = []tenant.Tenant{}
	}
	writeJSON(w, http.StatusOK, tenants)
}

// CreateTenant handles POST /api/v1/tenants
func (h *Handlers) CreateTenant(w http.ResponseWriter, r *http.Request) {
	req, ok := readJSON[tenant.CreateRequest](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}

	t, err := h.Tenants.Create(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

// GetTenant handles GET /api/v1/tenants/{id}
func (h *Handlers) GetTenant(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	t, err := h.Tenants.Get(r.Context(), id)
	if err != nil {
		writeDomainError(w, err, "tenant not found")
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// UpdateTenant handles PUT /api/v1/tenants/{id}
func (h *Handlers) UpdateTenant(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	req, ok := readJSON[tenant.UpdateRequest](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}

	t, err := h.Tenants.Update(r.Context(), id, req)
	if err != nil {
		writeDomainError(w, err, "tenant not found")
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// --- VCS Webhooks ---

// HandleGitHubWebhook handles POST /api/v1/webhooks/vcs/github
func (h *Handlers) HandleGitHubWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read body")
		return
	}

	eventType := r.Header.Get("X-GitHub-Event")
	switch eventType {
	case "push":
		ev, err := h.VCSWebhook.HandleGitHubPush(r.Context(), body)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, ev)
	case "pull_request":
		ev, err := h.VCSWebhook.HandleGitHubPullRequest(r.Context(), body)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, ev)
	default:
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored", "event": eventType})
	}
}

// HandleGitLabWebhook handles POST /api/v1/webhooks/vcs/gitlab
func (h *Handlers) HandleGitLabWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read body")
		return
	}

	eventType := r.Header.Get("X-Gitlab-Event")
	switch eventType {
	case "Push Hook":
		ev, err := h.VCSWebhook.HandleGitLabPush(r.Context(), body)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, ev)
	default:
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored", "event": eventType})
	}
}

// --- Bidirectional Sync ---

// SyncRoadmap handles POST /api/v1/projects/{id}/roadmap/sync
func (h *Handlers) SyncRoadmap(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	req, ok := readJSON[roadmap.SyncConfig](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	req.ProjectID = projectID

	if req.Provider == "" {
		writeError(w, http.StatusBadRequest, "provider is required")
		return
	}
	if req.ProjectRef == "" {
		writeError(w, http.StatusBadRequest, "project_ref is required")
		return
	}
	if req.Direction == "" {
		req.Direction = roadmap.SyncDirectionPull
	}

	result, err := h.Sync.Sync(r.Context(), req)
	if err != nil {
		writeDomainError(w, err, "sync failed")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// --- PM Webhooks ---

// HandleGitHubIssueWebhook handles POST /api/v1/webhooks/pm/github
func (h *Handlers) HandleGitHubIssueWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read body")
		return
	}

	eventType := r.Header.Get("X-GitHub-Event")
	if eventType != "issues" {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored", "event": eventType})
		return
	}

	ev, err := h.PMWebhook.HandleGitHubIssueWebhook(r.Context(), body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, ev)
}

// HandleGitLabIssueWebhook handles POST /api/v1/webhooks/pm/gitlab
func (h *Handlers) HandleGitLabIssueWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read body")
		return
	}

	eventType := r.Header.Get("X-Gitlab-Event")
	if eventType != "Issue Hook" {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored", "event": eventType})
		return
	}

	ev, err := h.PMWebhook.HandleGitLabIssueWebhook(r.Context(), body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, ev)
}

// HandlePlaneWebhook handles POST /api/v1/webhooks/pm/plane
func (h *Handlers) HandlePlaneWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read body")
		return
	}

	ev, err := h.PMWebhook.HandlePlaneWebhook(r.Context(), body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, ev)
}

// --- Review Policies & Reviews (Phase 12I) ---

// ListReviewPolicies handles GET /api/v1/projects/{id}/review-policies
func (h *Handlers) ListReviewPolicies(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	policies, err := h.Review.ListPolicies(r.Context(), projectID)
	if err != nil {
		writeDomainError(w, err, "project not found")
		return
	}
	writeJSON(w, http.StatusOK, policies)
}

// CreateReviewPolicy handles POST /api/v1/projects/{id}/review-policies
func (h *Handlers) CreateReviewPolicy(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	tenantID := middleware.TenantIDFromContext(r.Context())
	req, ok := readJSON[review.CreatePolicyRequest](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}

	p, err := h.Review.CreatePolicy(r.Context(), projectID, tenantID, &req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

// GetReviewPolicy handles GET /api/v1/review-policies/{id}
func (h *Handlers) GetReviewPolicy(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	p, err := h.Review.GetPolicy(r.Context(), id)
	if err != nil {
		writeDomainError(w, err, "review policy not found")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// UpdateReviewPolicy handles PUT /api/v1/review-policies/{id}
func (h *Handlers) UpdateReviewPolicy(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	req, ok := readJSON[review.UpdatePolicyRequest](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}

	p, err := h.Review.UpdatePolicy(r.Context(), id, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// DeleteReviewPolicy handles DELETE /api/v1/review-policies/{id}
func (h *Handlers) DeleteReviewPolicy(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.Review.DeletePolicy(r.Context(), id); err != nil {
		writeDomainError(w, err, "review policy not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// TriggerReview handles POST /api/v1/review-policies/{id}/trigger
func (h *Handlers) TriggerReview(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	rev, err := h.Review.ManualTrigger(r.Context(), id)
	if err != nil {
		writeDomainError(w, err, "review policy not found")
		return
	}
	writeJSON(w, http.StatusCreated, rev)
}

// ListReviews handles GET /api/v1/projects/{id}/reviews
func (h *Handlers) ListReviews(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	reviews, err := h.Review.ListReviews(r.Context(), projectID)
	if err != nil {
		writeDomainError(w, err, "project not found")
		return
	}
	writeJSON(w, http.StatusOK, reviews)
}

// GetReviewHandler handles GET /api/v1/reviews/{id}
func (h *Handlers) GetReviewHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	rev, err := h.Review.GetReview(r.Context(), id)
	if err != nil {
		writeDomainError(w, err, "review not found")
		return
	}
	writeJSON(w, http.StatusOK, rev)
}

// --- Settings ---

// GetSettings handles GET /api/v1/settings
func (h *Handlers) GetSettings(w http.ResponseWriter, r *http.Request) {
	list, err := h.Settings.List(r.Context())
	if err != nil {
		writeInternalError(w, err)
		return
	}

	// Return as a map of key -> value for frontend convenience.
	result := make(map[string]json.RawMessage, len(list))
	for _, s := range list {
		result[s.Key] = s.Value
	}
	writeJSON(w, http.StatusOK, result)
}

// UpdateSettings handles PUT /api/v1/settings
func (h *Handlers) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	req, ok := readJSON[settings.UpdateRequest](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	if len(req.Settings) == 0 {
		writeError(w, http.StatusBadRequest, "settings map must not be empty")
		return
	}
	if err := h.Settings.Update(r.Context(), req); err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// --- VCS Accounts ---

// ListVCSAccounts handles GET /api/v1/vcs-accounts
func (h *Handlers) ListVCSAccounts(w http.ResponseWriter, r *http.Request) {
	accounts, err := h.VCSAccounts.List(r.Context())
	if err != nil {
		writeInternalError(w, err)
		return
	}
	if accounts == nil {
		accounts = []vcsaccount.VCSAccount{}
	}
	writeJSON(w, http.StatusOK, accounts)
}

// CreateVCSAccount handles POST /api/v1/vcs-accounts
func (h *Handlers) CreateVCSAccount(w http.ResponseWriter, r *http.Request) {
	req, ok := readJSON[vcsaccount.CreateRequest](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	account, err := h.VCSAccounts.Create(r.Context(), &req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// Clear encrypted token from the response.
	account.EncryptedToken = nil
	writeJSON(w, http.StatusCreated, account)
}

// DeleteVCSAccount handles DELETE /api/v1/vcs-accounts/{id}
func (h *Handlers) DeleteVCSAccount(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.VCSAccounts.Delete(r.Context(), id); err != nil {
		writeDomainError(w, err, "vcs account not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// TestVCSAccount handles POST /api/v1/vcs-accounts/{id}/test
func (h *Handlers) TestVCSAccount(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.VCSAccounts.Test(r.Context(), id); err != nil {
		writeDomainError(w, err, "vcs account not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// --- Conversation Handlers ---

// CreateConversation handles POST /api/v1/projects/{id}/conversations
