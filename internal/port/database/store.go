// Package database defines the database store port (interface).
package database

import (
	"context"

	"github.com/Strob0t/CodeForge/internal/domain/agent"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/task"
)

// Store is the port interface for database operations.
type Store interface {
	// Projects
	ListProjects(ctx context.Context) ([]project.Project, error)
	GetProject(ctx context.Context, id string) (*project.Project, error)
	CreateProject(ctx context.Context, req project.CreateRequest) (*project.Project, error)
	UpdateProject(ctx context.Context, p *project.Project) error
	DeleteProject(ctx context.Context, id string) error

	// Agents
	ListAgents(ctx context.Context, projectID string) ([]agent.Agent, error)
	GetAgent(ctx context.Context, id string) (*agent.Agent, error)
	CreateAgent(ctx context.Context, projectID, name, backend string, config map[string]string) (*agent.Agent, error)
	UpdateAgentStatus(ctx context.Context, id string, status agent.Status) error
	DeleteAgent(ctx context.Context, id string) error

	// Tasks
	ListTasks(ctx context.Context, projectID string) ([]task.Task, error)
	GetTask(ctx context.Context, id string) (*task.Task, error)
	CreateTask(ctx context.Context, req task.CreateRequest) (*task.Task, error)
	UpdateTaskStatus(ctx context.Context, id string, status task.Status) error
	UpdateTaskResult(ctx context.Context, id string, result task.Result, costUSD float64) error
}
