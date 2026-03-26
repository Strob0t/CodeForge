package database

import (
	"context"

	"github.com/Strob0t/CodeForge/internal/domain/project"
)

// ProjectStore defines database operations for project management.
type ProjectStore interface {
	ListProjects(ctx context.Context) ([]project.Project, error)
	GetProject(ctx context.Context, id string) (*project.Project, error)
	CreateProject(ctx context.Context, req *project.CreateRequest) (*project.Project, error)
	UpdateProject(ctx context.Context, p *project.Project) error
	DeleteProject(ctx context.Context, id string) error
	GetProjectByRepoName(ctx context.Context, repoName string) (*project.Project, error)
	BatchDeleteProjects(ctx context.Context, ids []string) ([]string, error)
	BatchGetProjects(ctx context.Context, ids []string) ([]project.Project, error)
}
