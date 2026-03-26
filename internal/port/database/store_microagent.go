package database

import (
	"context"

	"github.com/Strob0t/CodeForge/internal/domain/microagent"
)

// MicroagentStore defines database operations for microagent management.
type MicroagentStore interface {
	CreateMicroagent(ctx context.Context, m *microagent.Microagent) error
	GetMicroagent(ctx context.Context, id string) (*microagent.Microagent, error)
	ListMicroagents(ctx context.Context, projectID string) ([]microagent.Microagent, error)
	UpdateMicroagent(ctx context.Context, m *microagent.Microagent) error
	DeleteMicroagent(ctx context.Context, id string) error
}
