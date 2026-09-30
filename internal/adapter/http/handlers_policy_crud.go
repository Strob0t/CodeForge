package http

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/policy"
	"github.com/Strob0t/CodeForge/internal/service"
)

// PolicyHandlers groups HTTP handlers for policy profile CRUD,
// evaluation, and the allow-always mechanism. Persistence of custom
// profiles is handled by the PolicyService (see LoadPolicyDir).
type PolicyHandlers struct {
	Policies *service.PolicyService
	Projects *service.ProjectService
	Limits   *config.Limits
}

// ListPolicyProfiles handles GET /api/v1/policies
func (ph *PolicyHandlers) ListPolicyProfiles(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string][]string{
		"profiles": ph.Policies.ListProfiles(),
	})
}

// GetPolicyProfile handles GET /api/v1/policies/{name}
func (ph *PolicyHandlers) GetPolicyProfile(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	p, ok := ph.Policies.GetProfile(name)
	if !ok {
		writeError(w, http.StatusNotFound, "policy profile not found")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// EvaluatePolicy handles POST /api/v1/policies/{name}/evaluate
func (ph *PolicyHandlers) EvaluatePolicy(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	call, ok := readJSON[policy.ToolCall](w, r, ph.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	if call.Tool == "" {
		writeError(w, http.StatusBadRequest, "tool is required")
		return
	}

	result, err := ph.Policies.EvaluateWithReason(r.Context(), name, call)
	if err != nil {
		writeDomainError(w, err, "policy not found")
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// CreatePolicyProfile handles POST /api/v1/policies
func (ph *PolicyHandlers) CreatePolicyProfile(w http.ResponseWriter, r *http.Request) {
	profile, ok := readJSON[policy.PolicyProfile](w, r, ph.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	if err := sanitizeName(profile.Name); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := ph.Policies.SaveProfile(&profile); err != nil {
		switch {
		case errors.Is(err, domain.ErrConflict) && policy.IsPreset(profile.Name):
			writeError(w, http.StatusConflict, "built-in policy presets cannot be overwritten")
			return
		case errors.Is(err, domain.ErrConflict):
			writeError(w, http.StatusConflict, "a policy file with this profile name already exists in the policy directory")
			return
		}
		writeDomainError(w, err, "save policy profile failed")
		return
	}

	writeJSON(w, http.StatusCreated, profile)
}

// DeletePolicyProfile handles DELETE /api/v1/policies/{name}
func (ph *PolicyHandlers) DeletePolicyProfile(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if err := sanitizeName(name); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := ph.Policies.DeleteProfile(name); err != nil {
		switch {
		case policy.IsPreset(name):
			writeError(w, http.StatusForbidden, err.Error())
		case errors.Is(err, domain.ErrNotFound):
			writeError(w, http.StatusNotFound, err.Error())
		default:
			slog.Error("failed to delete policy profile", "name", name, "error", err)
			writeError(w, http.StatusInternalServerError, "delete policy profile failed")
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// AllowAlwaysPolicy handles POST /api/v1/policies/allow-always.
// It delegates to PolicyService.AllowAlways which handles profile cloning,
// rule construction, and filesystem persistence. profile is the policy
// profile named by the permission request (optional).
func (ph *PolicyHandlers) AllowAlwaysPolicy(w http.ResponseWriter, r *http.Request) {
	req, ok := readJSON[struct {
		ProjectID string `json:"project_id"`
		Profile   string `json:"profile,omitempty"`
		Tool      string `json:"tool"`
		Command   string `json:"command,omitempty"`
	}](w, r, 1<<20)
	if !ok {
		return
	}
	if req.ProjectID == "" {
		writeError(w, http.StatusBadRequest, "project_id is required")
		return
	}
	if req.Tool == "" {
		writeError(w, http.StatusBadRequest, "tool is required")
		return
	}

	result, err := ph.Policies.AllowAlways(r.Context(), ph.Projects, req.ProjectID, req.Profile, req.Tool, req.Command)
	if err != nil {
		if errors.Is(err, service.ErrPolicyDirNotConfigured) {
			writeError(w, http.StatusConflict, "allow-always rules cannot be persisted: "+err.Error())
			return
		}
		writeDomainError(w, err, "allow-always failed")
		return
	}
	writeJSON(w, http.StatusOK, result)
}
