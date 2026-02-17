package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/Strob0t/CodeForge/internal/adapter/ws"
	cfcontext "github.com/Strob0t/CodeForge/internal/domain/context"
	"github.com/Strob0t/CodeForge/internal/port/broadcast"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// SharedContextService manages team-level shared context for collaboration.
type SharedContextService struct {
	store database.Store
	hub   broadcast.Broadcaster
	queue messagequeue.Queue
}

// NewSharedContextService creates a SharedContextService with all dependencies.
func NewSharedContextService(
	store database.Store,
	hub broadcast.Broadcaster,
	queue messagequeue.Queue,
) *SharedContextService {
	return &SharedContextService{store: store, hub: hub, queue: queue}
}

// InitForTeam creates a new empty shared context for a team.
func (s *SharedContextService) InitForTeam(ctx context.Context, teamID, projectID string) (*cfcontext.SharedContext, error) {
	sc := &cfcontext.SharedContext{
		TeamID:    teamID,
		ProjectID: projectID,
	}
	if err := sc.Validate(); err != nil {
		return nil, fmt.Errorf("validate shared context: %w", err)
	}
	if err := s.store.CreateSharedContext(ctx, sc); err != nil {
		return nil, fmt.Errorf("create shared context: %w", err)
	}
	slog.Info("shared context initialized", "team_id", teamID, "shared_id", sc.ID)
	return sc, nil
}

// AddItem adds a key-value pair to the team's shared context and notifies via NATS.
func (s *SharedContextService) AddItem(ctx context.Context, req cfcontext.AddSharedItemRequest) (*cfcontext.SharedContextItem, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("validate: %w", err)
	}

	item, err := s.store.AddSharedContextItem(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("add shared item: %w", err)
	}

	// Publish NATS notification.
	if s.queue != nil {
		payload := messagequeue.SharedContextUpdatedPayload{
			TeamID:  req.TeamID,
			Key:     req.Key,
			Author:  req.Author,
			Version: 0, // version is incremented in store; we don't re-read here
		}
		data, err := json.Marshal(payload)
		if err == nil {
			if err := s.queue.Publish(ctx, messagequeue.SubjectSharedUpdated, data); err != nil {
				slog.Warn("failed to publish shared context update", "team_id", req.TeamID, "error", err)
			}
		}
	}

	// Broadcast via WebSocket for real-time frontend updates.
	if s.hub != nil {
		s.hub.BroadcastEvent(ctx, ws.EventSharedContextUpdate, ws.SharedContextUpdateEvent{
			TeamID: req.TeamID,
			Key:    req.Key,
			Author: req.Author,
		})
	}

	slog.Info("shared context item added", "team_id", req.TeamID, "key", req.Key, "author", req.Author)
	return item, nil
}

// Get returns the shared context for a team.
func (s *SharedContextService) Get(ctx context.Context, teamID string) (*cfcontext.SharedContext, error) {
	return s.store.GetSharedContextByTeam(ctx, teamID)
}
