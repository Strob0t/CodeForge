package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/Strob0t/CodeForge/internal/adapter/ws"
	"github.com/Strob0t/CodeForge/internal/config"
	cfcontext "github.com/Strob0t/CodeForge/internal/domain/context"
	"github.com/Strob0t/CodeForge/internal/port/broadcast"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// RepoMapService manages repository map generation and retrieval.
type RepoMapService struct {
	store   database.Store
	queue   messagequeue.Queue
	hub     broadcast.Broadcaster
	orchCfg *config.Orchestrator
}

// NewRepoMapService creates a RepoMapService.
func NewRepoMapService(store database.Store, queue messagequeue.Queue, hub broadcast.Broadcaster, orchCfg *config.Orchestrator) *RepoMapService {
	return &RepoMapService{store: store, queue: queue, hub: hub, orchCfg: orchCfg}
}

// RequestGeneration publishes a request for repo map generation to the Python worker.
func (s *RepoMapService) RequestGeneration(ctx context.Context, projectID string, activeFiles []string) error {
	proj, err := s.store.GetProject(ctx, projectID)
	if err != nil {
		return fmt.Errorf("get project: %w", err)
	}

	budget := s.orchCfg.RepoMapTokenBudget
	if budget <= 0 {
		budget = 1024
	}

	payload := messagequeue.RepoMapRequestPayload{
		ProjectID:     projectID,
		WorkspacePath: proj.WorkspacePath,
		TokenBudget:   budget,
		ActiveFiles:   activeFiles,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal repomap request: %w", err)
	}

	if err := s.queue.Publish(ctx, messagequeue.SubjectRepoMapRequest, data); err != nil {
		return fmt.Errorf("publish repomap request: %w", err)
	}

	s.hub.BroadcastEvent(ctx, ws.EventRepoMapStatus, ws.RepoMapStatusEvent{
		ProjectID: projectID,
		Status:    "generating",
	})

	slog.Info("repomap generation requested", "project_id", projectID)
	return nil
}

// HandleResult processes the result of a repo map generation from the Python worker.
func (s *RepoMapService) HandleResult(ctx context.Context, payload *messagequeue.RepoMapResultPayload) error {
	if payload.Error != "" {
		slog.Error("repomap generation failed", "project_id", payload.ProjectID, "error", payload.Error)
		s.hub.BroadcastEvent(ctx, ws.EventRepoMapStatus, ws.RepoMapStatusEvent{
			ProjectID: payload.ProjectID,
			Status:    "failed",
			Error:     payload.Error,
		})
		return nil
	}

	m := &cfcontext.RepoMap{
		ProjectID:   payload.ProjectID,
		MapText:     payload.MapText,
		TokenCount:  payload.TokenCount,
		FileCount:   payload.FileCount,
		SymbolCount: payload.SymbolCount,
		Languages:   payload.Languages,
	}

	if err := s.store.UpsertRepoMap(ctx, m); err != nil {
		return fmt.Errorf("upsert repomap: %w", err)
	}

	s.hub.BroadcastEvent(ctx, ws.EventRepoMapStatus, ws.RepoMapStatusEvent{
		ProjectID:   payload.ProjectID,
		Status:      "ready",
		TokenCount:  payload.TokenCount,
		FileCount:   payload.FileCount,
		SymbolCount: payload.SymbolCount,
	})

	slog.Info("repomap stored",
		"project_id", payload.ProjectID,
		"tokens", payload.TokenCount,
		"files", payload.FileCount,
		"symbols", payload.SymbolCount,
	)
	return nil
}

// Get returns the stored repo map for a project.
func (s *RepoMapService) Get(ctx context.Context, projectID string) (*cfcontext.RepoMap, error) {
	return s.store.GetRepoMap(ctx, projectID)
}

// StartSubscriber subscribes to repomap.generate.result and processes incoming results.
func (s *RepoMapService) StartSubscriber(ctx context.Context) (cancel func(), err error) {
	return s.queue.Subscribe(ctx, messagequeue.SubjectRepoMapResult, func(msgCtx context.Context, _ string, data []byte) error {
		var payload messagequeue.RepoMapResultPayload
		if err := json.Unmarshal(data, &payload); err != nil {
			return fmt.Errorf("unmarshal repomap result: %w", err)
		}
		return s.HandleResult(msgCtx, &payload)
	})
}
