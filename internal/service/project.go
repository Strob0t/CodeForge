// Package service implements business logic on top of ports.
package service

import (
	"context"

	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

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
