package http

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/middleware"
	"github.com/Strob0t/CodeForge/internal/port/gitprovider"
	"github.com/Strob0t/CodeForge/internal/service"
)

// ProjectHandlers groups HTTP handlers for project CRUD, git operations,
// workspace management, and stack detection.
type ProjectHandlers struct {
	Projects *service.ProjectService
	Limits   *config.Limits
}

// ListProjects handles GET /api/v1/projects
// Supports ?limit=N&offset=N query params (default limit=100, offset=0).
func (ph *ProjectHandlers) ListProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := ph.Projects.List(r.Context())
	if err != nil {
		writeInternalError(w, err)
		return
	}
	if projects == nil {
		projects = []project.Project{}
	}
	limit, offset := parsePagination(r, 100)
	projects = applyPagination(projects, limit, offset)
	writeJSON(w, http.StatusOK, projects)
}

// GetProject handles GET /api/v1/projects/{id}
func (ph *ProjectHandlers) GetProject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	p, err := ph.Projects.Get(r.Context(), id)
	if err != nil {
		writeDomainError(w, err, "project not found")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// CreateProject handles POST /api/v1/projects
func (ph *ProjectHandlers) CreateProject(w http.ResponseWriter, r *http.Request) {
	req, ok := readJSON[project.CreateRequest](w, r, ph.Limits.MaxRequestBodySize)
	if !ok {
		return
	}

	if err := project.ValidateCreateRequest(&req, gitprovider.Available()); err != nil {
		writeDomainError(w, err, "invalid project request")
		return
	}

	p, err := ph.Projects.Create(r.Context(), &req)
	if err != nil {
		writeDomainError(w, err, "project creation failed")
		return
	}

	// If local_path provided, adopt the workspace in the same request.
	if req.LocalPath != "" {
		adopted, adoptErr := ph.Projects.Adopt(r.Context(), p.ID, req.LocalPath)
		if adoptErr != nil {
			writeDomainError(w, adoptErr, "project created but workspace adoption failed")
			return
		}
		p = adopted
		ph.autoIndexProject(middleware.TenantIDFromContext(r.Context()), p.ID, p.WorkspacePath)
	}

	writeJSON(w, http.StatusCreated, p)
}

// DeleteProject handles DELETE /api/v1/projects/{id}
func (ph *ProjectHandlers) DeleteProject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := ph.Projects.Delete(r.Context(), id); err != nil {
		writeDomainError(w, err, "project not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// UpdateProject handles PUT /api/v1/projects/{id}
func (ph *ProjectHandlers) UpdateProject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	req, ok := readJSON[project.UpdateRequest](w, r, ph.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	p, err := ph.Projects.Update(r.Context(), id, req)
	if err != nil {
		writeDomainError(w, err, "project not found")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// ParseRepoURL handles POST /api/v1/parse-repo-url
func (ph *ProjectHandlers) ParseRepoURL(w http.ResponseWriter, r *http.Request) {
	req, ok := readJSON[struct {
		URL string `json:"url"`
	}](w, r, 1<<20)
	if !ok {
		return
	}
	if req.URL == "" {
		writeError(w, http.StatusBadRequest, "url is required")
		return
	}
	parsed, err := project.ParseRepoURL(req.URL)
	if err != nil {
		writeDomainError(w, err, "invalid repository URL")
		return
	}
	writeJSON(w, http.StatusOK, parsed)
}

// FetchRepoInfo handles GET /api/v1/repos/info?url=<repo_url>
// It queries the hosting platform's API to fetch repository metadata.
func (ph *ProjectHandlers) FetchRepoInfo(w http.ResponseWriter, r *http.Request) {
	repoURL := r.URL.Query().Get("url")
	if repoURL == "" {
		writeError(w, http.StatusBadRequest, "url query parameter is required")
		return
	}
	info, err := ph.Projects.FetchRepoInfo(r.Context(), repoURL)
	if err != nil {
		writeError(w, http.StatusBadGateway, "repository unreachable")
		return
	}
	writeJSON(w, http.StatusOK, info)
}

// autoIndexProject delegates background indexing to the ProjectService.
func (ph *ProjectHandlers) autoIndexProject(tenantID, projectID, workspacePath string) {
	ph.Projects.AutoIndex(tenantID, projectID, workspacePath)
}

// CloneProject handles POST /api/v1/projects/{id}/clone
func (ph *ProjectHandlers) CloneProject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	tenantID := middleware.TenantIDFromContext(r.Context())

	// Optionally accept a branch in the request body.
	var body struct {
		Branch string `json:"branch"`
	}
	// Ignore decode errors — body is optional for backward compatibility.
	_ = json.NewDecoder(r.Body).Decode(&body)

	p, err := ph.Projects.Clone(r.Context(), id, tenantID, body.Branch)
	if err != nil {
		writeDomainError(w, err, "clone failed")
		return
	}

	ph.autoIndexProject(tenantID, id, p.WorkspacePath)

	writeJSON(w, http.StatusOK, p)
}

// AdoptProject handles POST /api/v1/projects/{id}/adopt
func (ph *ProjectHandlers) AdoptProject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	req, ok := readJSON[project.AdoptRequest](w, r, ph.Limits.MaxRequestBodySize)
	if !ok {
		return
	}

	// Validate the path is an absolute path and exists
	if req.Path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	cleanPath := filepath.Clean(req.Path)
	if !filepath.IsAbs(cleanPath) {
		writeError(w, http.StatusBadRequest, "path must be absolute")
		return
	}
	// Prevent traversal: path must resolve to itself after cleaning
	if cleanPath != req.Path && cleanPath+"/" != req.Path {
		writeError(w, http.StatusBadRequest, "path contains invalid characters")
		return
	}

	p, err := ph.Projects.Adopt(r.Context(), id, cleanPath)
	if err != nil {
		writeDomainError(w, err, "adopt failed")
		return
	}

	ph.autoIndexProject(middleware.TenantIDFromContext(r.Context()), id, p.WorkspacePath)

	writeJSON(w, http.StatusOK, p)
}

// InitWorkspace handles POST /api/v1/projects/{id}/init-workspace
// It creates an empty workspace directory with git init for a project without a repo URL.
func (ph *ProjectHandlers) InitWorkspace(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	tenantID := middleware.TenantIDFromContext(r.Context())

	p, err := ph.Projects.InitWorkspace(r.Context(), id, tenantID)
	if err != nil {
		writeDomainError(w, err, "init workspace failed")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// SetupProject handles POST /api/v1/projects/{id}/setup
// It chains clone, stack detection, and spec import in a single request.
func (ph *ProjectHandlers) SetupProject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	tenantID := middleware.TenantIDFromContext(r.Context())

	// Optionally accept a branch in the request body.
	var body struct {
		Branch string `json:"branch"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	result, err := ph.Projects.SetupProject(r.Context(), id, tenantID, body.Branch)
	if err != nil {
		writeDomainError(w, err, "setup failed")
		return
	}

	// Trigger background indexing if the project now has a workspace.
	if p, pErr := ph.Projects.Get(r.Context(), id); pErr == nil && p.WorkspacePath != "" {
		ph.autoIndexProject(tenantID, id, p.WorkspacePath)
	}

	writeJSON(w, http.StatusOK, result)
}

// GetWorkspaceInfo handles GET /api/v1/projects/{id}/workspace
func (ph *ProjectHandlers) GetWorkspaceInfo(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	info, err := ph.Projects.WorkspaceHealth(r.Context(), id)
	if err != nil {
		writeDomainError(w, err, "workspace info failed")
		return
	}

	writeJSON(w, http.StatusOK, info)
}

// DetectProjectStack handles GET /api/v1/projects/{id}/detect-stack
func (ph *ProjectHandlers) DetectProjectStack(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	result, err := ph.Projects.DetectStack(r.Context(), id)
	if err != nil {
		writeDomainError(w, err, "stack detection failed")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// DetectStackByPath handles POST /api/v1/detect-stack
func (ph *ProjectHandlers) DetectStackByPath(w http.ResponseWriter, r *http.Request) {
	req, ok := readJSON[struct {
		Path string `json:"path"`
	}](w, r, ph.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	if req.Path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	cleanPath := filepath.Clean(req.Path)
	if !filepath.IsAbs(cleanPath) {
		writeError(w, http.StatusBadRequest, "path must be absolute")
		return
	}
	result, err := ph.Projects.DetectStackByPath(r.Context(), cleanPath)
	if err != nil {
		writeDomainError(w, err, "stack detection failed")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// ProjectGitStatus handles GET /api/v1/projects/{id}/git/status
func (ph *ProjectHandlers) ProjectGitStatus(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	status, err := ph.Projects.Status(r.Context(), id)
	if err != nil {
		writeDomainError(w, err, "project not found")
		return
	}
	writeJSON(w, http.StatusOK, status)
}

// PullProject handles POST /api/v1/projects/{id}/git/pull
func (ph *ProjectHandlers) PullProject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := ph.Projects.Pull(r.Context(), id); err != nil {
		writeDomainError(w, err, "project not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ListProjectBranches handles GET /api/v1/projects/{id}/git/branches
func (ph *ProjectHandlers) ListProjectBranches(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	branches, err := ph.Projects.ListBranches(r.Context(), id)
	if err != nil {
		writeDomainError(w, err, "project not found")
		return
	}
	writeJSON(w, http.StatusOK, branches)
}

// CheckoutBranch handles POST /api/v1/projects/{id}/git/checkout
func (ph *ProjectHandlers) CheckoutBranch(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	req, ok := readJSON[struct {
		Branch string `json:"branch"`
	}](w, r, ph.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	if req.Branch == "" {
		writeError(w, http.StatusBadRequest, "branch is required")
		return
	}

	if err := ph.Projects.Checkout(r.Context(), id, req.Branch); err != nil {
		writeDomainError(w, err, "project not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "branch": req.Branch})
}

// ListRemoteBranches handles GET /api/v1/projects/remote-branches?url=<repo-url>
// Delegates to ProjectService.ListRemoteBranches for URL validation and git operations.
func (ph *ProjectHandlers) ListRemoteBranches(w http.ResponseWriter, r *http.Request) {
	repoURL := r.URL.Query().Get("url")
	if repoURL == "" {
		writeError(w, http.StatusBadRequest, "url query parameter is required")
		return
	}

	branches, err := ph.Projects.ListRemoteBranches(r.Context(), repoURL)
	if err != nil {
		// Distinguish validation errors from git failures.
		errMsg := err.Error()
		if strings.Contains(errMsg, "invalid repository URL") ||
			strings.Contains(errMsg, "unsupported URL scheme") ||
			strings.Contains(errMsg, "url is required") {
			writeError(w, http.StatusBadRequest, errMsg)
			return
		}
		writeError(w, http.StatusBadGateway, "failed to list remote branches")
		return
	}

	if branches == nil {
		branches = []string{}
	}
	writeJSON(w, http.StatusOK, map[string][]string{"branches": branches})
}
