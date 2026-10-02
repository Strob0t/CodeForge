package http

import (
	"encoding/json"
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

// tenantWithToolUID is a tenant as platform admins see it: with its tool
// UID (KI-96), null while it has none. Operators need it for the setfacl
// command of an adopted workspace.
type tenantWithToolUID struct {
	tenant.Tenant
	ToolUID *int `json:"tool_uid"`
}

// writeTenant writes t, with its tool UID for platform admins only.
func writeTenant(w http.ResponseWriter, r *http.Request, status int, t *tenant.Tenant) {
	if isPlatformAdmin(r) {
		writeJSON(w, status, tenantWithToolUID{Tenant: *t, ToolUID: t.ToolUID})
		return
	}
	writeJSON(w, status, t)
}

// ListTenants handles GET /api/v1/tenants
func (h *Handlers) ListTenants(w http.ResponseWriter, r *http.Request) {
	tenants, err := h.Tenants.List(r.Context())
	if err != nil {
		writeInternalError(w, err)
		return
	}
	if !isPlatformAdmin(r) {
		writeJSONList(w, http.StatusOK, tenants)
		return
	}
	views := make([]tenantWithToolUID, 0, len(tenants))
	for i := range tenants {
		views = append(views, tenantWithToolUID{Tenant: tenants[i], ToolUID: tenants[i].ToolUID})
	}
	writeJSONList(w, http.StatusOK, views)
}

// CreateTenant handles POST /api/v1/tenants (platform admins only)
func (h *Handlers) CreateTenant(w http.ResponseWriter, r *http.Request) {
	req, ok := readJSON[tenant.CreateRequest](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}

	t, err := h.Tenants.Create(r.Context(), req)
	if err != nil {
		writeDomainError(w, err, "create tenant failed")
		return
	}
	writeTenant(w, r, http.StatusCreated, t)
}

// GetTenant handles GET /api/v1/tenants/{id}
func (h *Handlers) GetTenant(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	t, err := h.Tenants.Get(r.Context(), id)
	if err != nil {
		writeDomainError(w, err, "tenant not found")
		return
	}
	writeTenant(w, r, http.StatusOK, t)
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
	writeTenant(w, r, http.StatusOK, t)
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

	result, err := h.Sync.Sync(r.Context(), &req)
	if err != nil {
		writeDomainError(w, err, "sync failed")
		return
	}
	writeJSON(w, http.StatusOK, result)
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
		writeDomainError(w, err, "create review policy failed")
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
		writeDomainError(w, err, "update review policy failed")
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
	writeJSONList(w, http.StatusOK, accounts)
}

// CreateVCSAccount handles POST /api/v1/vcs-accounts
func (h *Handlers) CreateVCSAccount(w http.ResponseWriter, r *http.Request) {
	req, ok := readJSON[vcsaccount.CreateRequest](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	account, err := h.VCSAccounts.Create(r.Context(), &req)
	if err != nil {
		writeDomainError(w, err, "create vcs account failed")
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
