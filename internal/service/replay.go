package service

import (
	"context"
	"fmt"

	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/port/eventstore"
)

// ReplayService provides replay and audit trail capabilities.
type ReplayService struct {
	store  database.Store
	events eventstore.Store
}

// NewReplayService creates a new ReplayService.
func NewReplayService(store database.Store, events eventstore.Store) *ReplayService {
	return &ReplayService{store: store, events: events}
}

// ListCheckpoints returns checkpoint events for a run.
func (s *ReplayService) ListCheckpoints(ctx context.Context, runID string) ([]event.AgentEvent, error) {
	r, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("list checkpoints: %w", err)
	}
	return s.events.ListCheckpoints(ctx, r.ID)
}

// Replay returns events from a run, optionally filtered by event range.
func (s *ReplayService) Replay(ctx context.Context, req event.ReplayRequest) (*event.ReplayResult, error) {
	r, err := s.store.GetRun(ctx, req.RunID)
	if err != nil {
		return nil, fmt.Errorf("replay: %w", err)
	}

	events, err := s.events.LoadEventsRange(ctx, r.ID, req.FromEvent, req.ToEvent)
	if err != nil {
		return nil, fmt.Errorf("replay load events: %w", err)
	}

	return &event.ReplayResult{
		RunID:      r.ID,
		Events:     events,
		EventCount: len(events),
		DryRun:     req.DryRun,
	}, nil
}

// AuditTrail returns paginated audit entries with optional filtering.
func (s *ReplayService) AuditTrail(ctx context.Context, filter *event.AuditFilter, cursor string, limit int) (*event.AuditPage, error) {
	return s.events.LoadAudit(ctx, filter, cursor, limit)
}

// RecordAudit appends an audit trail entry.
func (s *ReplayService) RecordAudit(ctx context.Context, entry *event.AuditEntry) error {
	return s.events.AppendAudit(ctx, entry)
}
