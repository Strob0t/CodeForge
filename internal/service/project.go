// Package service implements business logic on top of ports.
package service

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/port/gitprovider"
)

// ProjectService handles project business logic.
type ProjectService struct {
	store         database.Store
	workspaceRoot string
}

// NewProjectService creates a new ProjectService.
func NewProjectService(store database.Store, workspaceRoot string) *ProjectService {
	return &ProjectService{store: store, workspaceRoot: workspaceRoot}
}

// List returns all projects.
func (s *ProjectService) List(ctx context.Context) ([]project.Project, error) {
	return s.store.ListProjects(ctx)
}

// Get returns a project by ID.
func (s *ProjectService) Get(ctx context.Context, id string) (*project.Project, error) {
	return s.store.GetProject(ctx, id)
}

// Create creates a new project after validating the request.
func (s *ProjectService) Create(ctx context.Context, req project.CreateRequest) (*project.Project, error) {
	if err := project.ValidateCreateRequest(req, gitprovider.Available()); err != nil {
		return nil, err
	}
	return s.store.CreateProject(ctx, req)
}

// Update applies partial updates to a project.
func (s *ProjectService) Update(ctx context.Context, id string, req project.UpdateRequest) (*project.Project, error) {
	if err := project.ValidateUpdateRequest(req); err != nil {
		return nil, err
	}

	p, err := s.store.GetProject(ctx, id)
	if err != nil {
		return nil, err
	}

	if req.Name != nil {
		p.Name = *req.Name
	}
	if req.Description != nil {
		p.Description = *req.Description
	}
	if req.RepoURL != nil {
		p.RepoURL = *req.RepoURL
	}
	if req.Provider != nil {
		p.Provider = *req.Provider
	}
	if req.Config != nil {
		p.Config = req.Config
	}

	if err := s.store.UpdateProject(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

// Delete removes a project and cleans up its workspace directory.
func (s *ProjectService) Delete(ctx context.Context, id string) error {
	p, err := s.store.GetProject(ctx, id)
	if err != nil {
		return s.store.DeleteProject(ctx, id)
	}

	wsPath := p.WorkspacePath

	if err := s.store.DeleteProject(ctx, id); err != nil {
		return err
	}

	if wsPath != "" && s.isUnderWorkspaceRoot(wsPath) {
		if rmErr := os.RemoveAll(wsPath); rmErr != nil {
			slog.Warn("failed to remove workspace directory",
				"project_id", id,
				"path", wsPath,
				"error", rmErr,
			)
		}
	}

	return nil
}

// Clone clones a project's repository to the workspace directory.
// The tenantID is used to isolate workspaces per tenant.
func (s *ProjectService) Clone(ctx context.Context, id, tenantID string) (*project.Project, error) {
	p, err := s.store.GetProject(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}
	if p.RepoURL == "" {
		return nil, fmt.Errorf("project %s has no repo_url", id)
	}

	provider, err := gitprovider.New(p.Provider, p.Config)
	if err != nil {
		return nil, fmt.Errorf("create git provider: %w", err)
	}

	destPath := filepath.Join(s.workspaceRoot, tenantID, p.ID)
	if err := provider.Clone(ctx, p.RepoURL, destPath); err != nil {
		return nil, fmt.Errorf("clone: %w", err)
	}

	p.WorkspacePath = destPath
	if err := s.store.UpdateProject(ctx, p); err != nil {
		return nil, fmt.Errorf("update project workspace: %w", err)
	}

	return p, nil
}

// Adopt sets an existing directory as the project's workspace without cloning.
func (s *ProjectService) Adopt(ctx context.Context, id, path string) (*project.Project, error) {
	if path == "" {
		return nil, fmt.Errorf("adopt: path is required")
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("adopt: resolve path: %w", err)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return nil, fmt.Errorf("adopt: directory does not exist: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("adopt: %s is not a directory", absPath)
	}

	p, err := s.store.GetProject(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}

	p.WorkspacePath = absPath
	if err := s.store.UpdateProject(ctx, p); err != nil {
		return nil, fmt.Errorf("update project workspace: %w", err)
	}

	return p, nil
}

// WorkspaceHealth returns health and status information about a project's workspace.
func (s *ProjectService) WorkspaceHealth(ctx context.Context, id string) (*project.WorkspaceInfo, error) {
	p, err := s.store.GetProject(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}

	info := &project.WorkspaceInfo{Path: p.WorkspacePath}
	if p.WorkspacePath == "" {
		return info, nil
	}

	stat, err := os.Stat(p.WorkspacePath)
	if err != nil {
		return info, nil
	}
	info.Exists = true
	info.LastModified = stat.ModTime()

	// Check for .git directory.
	if gitStat, gitErr := os.Stat(filepath.Join(p.WorkspacePath, ".git")); gitErr == nil && gitStat.IsDir() {
		info.GitRepo = true
	}

	// Compute disk usage.
	var totalSize int64
	_ = filepath.WalkDir(p.WorkspacePath, func(_ string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil //nolint:nilerr // skip unreadable entries
		}
		if !d.IsDir() {
			if fi, fiErr := d.Info(); fiErr == nil {
				totalSize += fi.Size()
			}
		}
		return nil
	})
	info.DiskUsageBytes = totalSize

	return info, nil
}

// Status returns the git status of a project's workspace.
func (s *ProjectService) Status(ctx context.Context, id string) (*project.GitStatus, error) {
	p, err := s.store.GetProject(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}
	if p.WorkspacePath == "" {
		return nil, fmt.Errorf("project %s has no workspace (not cloned)", id)
	}

	provider, err := gitprovider.New(p.Provider, p.Config)
	if err != nil {
		return nil, fmt.Errorf("create git provider: %w", err)
	}

	return provider.Status(ctx, p.WorkspacePath)
}

// Pull fetches and merges updates for a project's workspace.
func (s *ProjectService) Pull(ctx context.Context, id string) error {
	p, err := s.store.GetProject(ctx, id)
	if err != nil {
		return fmt.Errorf("get project: %w", err)
	}
	if p.WorkspacePath == "" {
		return fmt.Errorf("project %s has no workspace (not cloned)", id)
	}

	provider, err := gitprovider.New(p.Provider, p.Config)
	if err != nil {
		return fmt.Errorf("create git provider: %w", err)
	}

	return provider.Pull(ctx, p.WorkspacePath)
}

// ListBranches returns all branches of a project's workspace.
func (s *ProjectService) ListBranches(ctx context.Context, id string) ([]project.Branch, error) {
	p, err := s.store.GetProject(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}
	if p.WorkspacePath == "" {
		return nil, fmt.Errorf("project %s has no workspace (not cloned)", id)
	}

	provider, err := gitprovider.New(p.Provider, p.Config)
	if err != nil {
		return nil, fmt.Errorf("create git provider: %w", err)
	}

	return provider.ListBranches(ctx, p.WorkspacePath)
}

// Checkout switches a project's workspace to the specified branch.
func (s *ProjectService) Checkout(ctx context.Context, id, branch string) error {
	p, err := s.store.GetProject(ctx, id)
	if err != nil {
		return fmt.Errorf("get project: %w", err)
	}
	if p.WorkspacePath == "" {
		return fmt.Errorf("project %s has no workspace (not cloned)", id)
	}

	provider, err := gitprovider.New(p.Provider, p.Config)
	if err != nil {
		return fmt.Errorf("create git provider: %w", err)
	}

	return provider.Checkout(ctx, p.WorkspacePath, branch)
}

// DetectStack scans an existing project's workspace and returns stack detection results.
func (s *ProjectService) DetectStack(ctx context.Context, id string) (*project.StackDetectionResult, error) {
	p, err := s.store.GetProject(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}
	if p.WorkspacePath == "" {
		return nil, fmt.Errorf("project %s has no workspace (not cloned)", id)
	}
	return project.ScanWorkspace(p.WorkspacePath)
}

// DetectStackByPath scans an arbitrary directory path for language detection.
func (s *ProjectService) DetectStackByPath(_ context.Context, path string) (*project.StackDetectionResult, error) {
	if path == "" {
		return nil, fmt.Errorf("detect stack: path is required")
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("detect stack: resolve path: %w", err)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return nil, fmt.Errorf("detect stack: directory does not exist: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("detect stack: %s is not a directory", absPath)
	}

	return project.ScanWorkspace(absPath)
}

// isUnderWorkspaceRoot validates that the path is under the workspace root
// to prevent accidental deletion of unrelated directories.
func (s *ProjectService) isUnderWorkspaceRoot(path string) bool {
	absRoot, err := filepath.Abs(s.workspaceRoot)
	if err != nil {
		return false
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	return strings.HasPrefix(absPath, absRoot+string(filepath.Separator))
}
