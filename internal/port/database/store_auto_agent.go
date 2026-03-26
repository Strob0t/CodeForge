package database

import (
	"context"

	"github.com/Strob0t/CodeForge/internal/domain/autoagent"
)

// AutoAgentStore defines database operations for auto-agent management.
type AutoAgentStore interface {
	UpsertAutoAgent(ctx context.Context, aa *autoagent.AutoAgent) error
	GetAutoAgent(ctx context.Context, projectID string) (*autoagent.AutoAgent, error)
	UpdateAutoAgentStatus(ctx context.Context, projectID string, status autoagent.Status, errMsg string) error
	UpdateAutoAgentProgress(ctx context.Context, aa *autoagent.AutoAgent) error
	DeleteAutoAgent(ctx context.Context, projectID string) error
}
