package database

import (
	"context"

	a2adomain "github.com/Strob0t/CodeForge/internal/domain/a2a"
)

// A2AStore defines database operations for A2A tasks, remote agents, and push configs.
type A2AStore interface {
	// A2A Tasks (Phase 27)
	CreateA2ATask(ctx context.Context, t *a2adomain.A2ATask) error
	GetA2ATask(ctx context.Context, id string) (*a2adomain.A2ATask, error)
	UpdateA2ATask(ctx context.Context, t *a2adomain.A2ATask) error
	ListA2ATasks(ctx context.Context, filter *A2ATaskFilter) ([]a2adomain.A2ATask, int, error)
	DeleteA2ATask(ctx context.Context, id string) error

	// A2A Remote Agents (Phase 27)
	CreateRemoteAgent(ctx context.Context, a *a2adomain.RemoteAgent) error
	GetRemoteAgent(ctx context.Context, id string) (*a2adomain.RemoteAgent, error)
	ListRemoteAgents(ctx context.Context, tenantID string, enabledOnly bool) ([]a2adomain.RemoteAgent, error)
	UpdateRemoteAgent(ctx context.Context, a *a2adomain.RemoteAgent) error
	DeleteRemoteAgent(ctx context.Context, id string) error

	// A2A Push Configs (Phase 27)
	CreateA2APushConfig(ctx context.Context, taskID, url, token string) (string, error)
	GetA2APushConfig(ctx context.Context, id string) (taskID, url, token string, err error)
	ListA2APushConfigs(ctx context.Context, taskID string) ([]A2APushConfig, error)
	DeleteA2APushConfig(ctx context.Context, id string) error
	DeleteAllA2APushConfigs(ctx context.Context, taskID string) error
}
