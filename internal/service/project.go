// Package service implements business logic on top of ports.
package service

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/port/gitprovider"
)

// WorkspaceRoot is the base directory where repositories are cloned.
const WorkspaceRoot = "data/workspaces"

// ProjectService handles project business logic.
type ProjectService struct {
	store database.Store
}

// NewProjectService creates a new ProjectService.
func NewProjectService(store database.Store) *ProjectService {
	return &ProjectService{store: store}
}

// List returns all projects.
func (s *ProjectService) List(ctx context.Context) ([]project.Project, error) {
	return s.store.ListProjects(ctx)
}

// Get returns a project by ID.
func (s *ProjectService) Get(ctx context.Context, id string) (*project.Project, error) {
	return s.store.GetProject(ctx, id)
}

// Create creates a new project.
func (s *ProjectService) Create(ctx context.Context, req project.CreateRequest) (*project.Project, error) {
	return s.store.CreateProject(ctx, req)
}

// Delete removes a project.
func (s *ProjectService) Delete(ctx context.Context, id string) error {
	return s.store.DeleteProject(ctx, id)
}

// Clone clones a project's repository to the workspace directory.
func (s *ProjectService) Clone(ctx context.Context, id string) (*project.Project, error) {
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

	destPath := filepath.Join(WorkspaceRoot, p.ID)
	if err := provider.Clone(ctx, p.RepoURL, destPath); err != nil {
		return nil, fmt.Errorf("clone: %w", err)
	}

	p.WorkspacePath = destPath
	if err := s.store.UpdateProject(ctx, p); err != nil {
		return nil, fmt.Errorf("update project workspace: %w", err)
	}

	return p, nil
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
