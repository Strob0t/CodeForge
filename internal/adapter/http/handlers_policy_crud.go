package http

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/policy"
	"github.com/Strob0t/CodeForge/internal/service"
)

// PolicyHandlers groups HTTP handlers for policy profile CRUD,
// evaluation, and the allow-always mechanism.
type PolicyHandlers struct {
	Policies  *service.PolicyService
	Projects  *service.ProjectService
	PolicyDir string
	Limits    *config.Limits
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
		writeDomainError(w, err, "save policy profile failed")
		return
	}

	if ph.PolicyDir != "" {
		path := filepath.Join(ph.PolicyDir, profile.Name+".yaml")
		if err := os.MkdirAll(ph.PolicyDir, 0o750); err != nil {
			slog.Error("failed to create policy directory", "error", err)
		} else if err := policy.SaveToFile(path, &profile); err != nil {
			slog.Error("failed to persist policy profile", "name", profile.Name, "error", err)
		}
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
		if policy.IsPreset(name) {
			writeError(w, http.StatusForbidden, err.Error())
		} else {
			writeError(w, http.StatusNotFound, err.Error())
		}
		return
	}

	if ph.PolicyDir != "" {
		path := filepath.Join(ph.PolicyDir, name+".yaml")
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) { //nolint:gosec // path constructed from validated PolicyDir + sanitized name
			slog.Error("failed to remove policy file", "name", name, "error", err)
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

// AllowAlwaysPolicy handles POST /api/v1/policies/allow-always.
// It adds a persistent "allow" rule for a specific tool to a project's policy profile.
// If the project uses a built-in preset, a custom clone is created first.
func (ph *PolicyHandlers) AllowAlwaysPolicy(w http.ResponseWriter, r *http.Request) {
	req, ok := readJSON[struct {
		ProjectID string `json:"project_id"`
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

	ctx := r.Context()

	// Get the project to resolve its policy profile.
	proj, err := ph.Projects.Get(ctx, req.ProjectID)
	if err != nil {
		writeDomainError(w, err, "project not found")
		return
	}

	// Resolve effective profile (project-level or service default).
	effectiveProfile := ph.Policies.ResolveProfile("", proj.PolicyProfile)

	// If the resolved profile is a built-in preset, clone it to a custom profile.
	if policy.IsPreset(effectiveProfile) {
		source, _ := ph.Policies.GetProfile(effectiveProfile)
		cloneName := effectiveProfile + "-custom-" + req.ProjectID
		clone := source
		clone.Name = cloneName
		clone.Description = fmt.Sprintf("Custom clone of %s for project %s", effectiveProfile, req.ProjectID)

		// Check if clone already exists (from a previous "Allow Always" call).
		if _, exists := ph.Policies.GetProfile(cloneName); !exists {
			if err := ph.Policies.SaveProfile(&clone); err != nil {
				writeInternalError(w, err)
				return
			}
		}

		// Update the project to use the custom clone.
		if err := ph.Projects.SetPolicyProfile(ctx, req.ProjectID, cloneName); err != nil {
			writeInternalError(w, err)
			return
		}
		effectiveProfile = cloneName
	}

	// Construct the permission rule.
	spec := policy.ToolSpecifier{Tool: req.Tool}
	if req.Command != "" {
		// Use first word as command prefix pattern (e.g., "git" from "git status").
		parts := strings.SplitN(req.Command, " ", 2)
		spec.SubPattern = parts[0] + "*"
	}
	rule := policy.PermissionRule{
		Specifier: spec,
		Decision:  policy.DecisionAllow,
	}

	// Prepend the rule (idempotent -- no-op if same specifier already exists).
	if err := ph.Policies.PrependRule(effectiveProfile, &rule); err != nil {
		writeInternalError(w, err)
		return
	}

	// Persist to disk if PolicyDir is configured.
	if ph.PolicyDir != "" {
		updated, ok := ph.Policies.GetProfile(effectiveProfile)
		if ok {
			path := filepath.Join(ph.PolicyDir, effectiveProfile+".yaml")
			if err := os.MkdirAll(ph.PolicyDir, 0o750); err != nil {
				slog.Error("failed to create policy directory", "error", err)
			} else if err := policy.SaveToFile(path, &updated); err != nil {
				slog.Error("failed to persist policy profile", "name", effectiveProfile, "error", err)
			}
		}
	}

	// Return the updated profile.
	updated, _ := ph.Policies.GetProfile(effectiveProfile)
	writeJSON(w, http.StatusOK, updated)
}
