package database

import (
	"context"

	"github.com/Strob0t/CodeForge/internal/domain/experience"
	"github.com/Strob0t/CodeForge/internal/domain/memory"
)

// MemoryStore defines database operations for agent memories and experience pool.
type MemoryStore interface {
	// Agent Memories (Phase 22B)
	CreateMemory(ctx context.Context, m *memory.Memory) error
	ListMemories(ctx context.Context, projectID string) ([]memory.Memory, error)

	// Experience Pool (Phase 22B)
	CreateExperienceEntry(ctx context.Context, e *experience.Entry) error
	GetExperienceEntry(ctx context.Context, id string) (*experience.Entry, error)
	ListExperienceEntries(ctx context.Context, projectID string) ([]experience.Entry, error)
	DeleteExperienceEntry(ctx context.Context, id string) error
	UpdateExperienceHit(ctx context.Context, id string) error
}
