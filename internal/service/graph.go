package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/Strob0t/CodeForge/internal/adapter/ws"
	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/port/broadcast"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// GraphStatusInfo holds the in-memory state of a project's graph.
type GraphStatusInfo struct {
	ProjectID string   `json:"project_id"`
	Status    string   `json:"status"` // "building", "ready", "error"
	NodeCount int      `json:"node_count"`
	EdgeCount int      `json:"edge_count"`
	Languages []string `json:"languages"`
	Error     string   `json:"error,omitempty"`
}

// GraphService manages code graph building and search via Python workers.
type GraphService struct {
	store   database.Store
	queue   messagequeue.Queue
	hub     broadcast.Broadcaster
	orchCfg *config.Orchestrator
	limits  *config.Limits

	mu     sync.RWMutex
	graphs map[string]*GraphStatusInfo

	searchWaiter *syncWaiter[messagequeue.GraphSearchResultPayload]

	healthMu          sync.Mutex
	lastSearchFailure time.Time
}

// NewGraphService creates a GraphService.
func NewGraphService(store database.Store, queue messagequeue.Queue, hub broadcast.Broadcaster, orchCfg *config.Orchestrator, limits *config.Limits) *GraphService {
	return &GraphService{
		store:        store,
		queue:        queue,
		hub:          hub,
		orchCfg:      orchCfg,
		limits:       limits,
		graphs:       make(map[string]*GraphStatusInfo),
		searchWaiter: newSyncWaiter[messagequeue.GraphSearchResultPayload]("graph-search"),
	}
}

// RequestBuild publishes a graph build request to the Python worker.
func (s *GraphService) RequestBuild(ctx context.Context, projectID, workspacePath string) error {
	payload := messagequeue.GraphBuildRequestPayload{
		ProjectID:     projectID,
		WorkspacePath: workspacePath,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal graph build request: %w", err)
	}

	if err := s.queue.Publish(ctx, messagequeue.SubjectGraphBuildRequest, data); err != nil {
		return fmt.Errorf("publish graph build request: %w", err)
	}

	s.mu.Lock()
	s.graphs[projectID] = &GraphStatusInfo{
		ProjectID: projectID,
		Status:    "building",
	}
	s.mu.Unlock()

	s.hub.BroadcastEvent(ctx, ws.EventGraphStatus, ws.GraphStatusEvent{
		ProjectID: projectID,
		Status:    "building",
	})

	slog.Info("graph build requested", "project_id", projectID)
	return nil
}

// HandleBuildResult processes the result of a graph build from the Python worker.
func (s *GraphService) HandleBuildResult(ctx context.Context, payload *messagequeue.GraphBuildResultPayload) error {
	status := payload.Status
	if payload.Error != "" {
		status = "error"
	}

	s.mu.Lock()
	s.graphs[payload.ProjectID] = &GraphStatusInfo{
		ProjectID: payload.ProjectID,
		Status:    status,
		NodeCount: payload.NodeCount,
		EdgeCount: payload.EdgeCount,
		Languages: payload.Languages,
		Error:     payload.Error,
	}
	s.mu.Unlock()

	s.hub.BroadcastEvent(ctx, ws.EventGraphStatus, ws.GraphStatusEvent{
		ProjectID: payload.ProjectID,
		Status:    status,
		NodeCount: payload.NodeCount,
		EdgeCount: payload.EdgeCount,
		Languages: payload.Languages,
		Error:     payload.Error,
	})

	if payload.Error != "" {
		slog.Error("graph build failed", "project_id", payload.ProjectID, "error", payload.Error)
	} else {
		slog.Info("graph build ready",
			"project_id", payload.ProjectID,
			"nodes", payload.NodeCount,
			"edges", payload.EdgeCount,
		)
	}
	return nil
}

// SearchSync sends a graph search request and waits synchronously for the result.
// scopeID is optional — set when the search originates from a scope fan-out (observability).
func (s *GraphService) SearchSync(ctx context.Context, projectID string, seedSymbols []string, maxHops, topK int, scopeID ...string) (*messagequeue.GraphSearchResultPayload, error) {
	if s.isUnhealthy() {
		return nil, fmt.Errorf("graph search skipped: worker recently unhealthy (project %s)", projectID)
	}

	requestID, err := generateRequestID()
	if err != nil {
		return nil, err
	}

	ch := s.searchWaiter.register(requestID)
	defer s.searchWaiter.unregister(requestID)

	payload := messagequeue.GraphSearchRequestPayload{
		ProjectID:   projectID,
		RequestID:   requestID,
		SeedSymbols: seedSymbols,
		MaxHops:     maxHops,
		TopK:        topK,
	}
	if len(scopeID) > 0 {
		payload.ScopeID = scopeID[0]
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal graph search request: %w", err)
	}

	if err := s.queue.Publish(ctx, messagequeue.SubjectGraphSearchRequest, data); err != nil {
		return nil, fmt.Errorf("publish graph search request: %w", err)
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, s.limits.GraphSearchTimeout)
	defer cancel()

	select {
	case result := <-ch:
		if result.Error != "" {
			s.recordFailure()
			return nil, fmt.Errorf("graph search error: %s", result.Error)
		}
		return result, nil
	case <-timeoutCtx.Done():
		s.recordFailure()
		return nil, fmt.Errorf("graph search timeout for project %s", projectID)
	}
}

// HandleSearchResult delivers a graph search result to the waiting caller.
func (s *GraphService) HandleSearchResult(_ context.Context, payload *messagequeue.GraphSearchResultPayload) {
	s.searchWaiter.deliver(payload.RequestID, payload)
}

// GetStatus returns the in-memory graph status for a project, or nil if unknown.
func (s *GraphService) GetStatus(projectID string) *GraphStatusInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.graphs[projectID]
}

// StartSubscribers subscribes to graph result subjects and returns cancel funcs.
func (s *GraphService) StartSubscribers(ctx context.Context) ([]func(), error) {
	cancelBuild, err := s.queue.Subscribe(ctx, messagequeue.SubjectGraphBuildResult, func(msgCtx context.Context, _ string, data []byte) error {
		var payload messagequeue.GraphBuildResultPayload
		if err := json.Unmarshal(data, &payload); err != nil {
			return fmt.Errorf("unmarshal graph build result: %w", err)
		}
		return s.HandleBuildResult(msgCtx, &payload)
	})
	if err != nil {
		return nil, fmt.Errorf("subscribe graph build result: %w", err)
	}

	cancelSearch, err := s.queue.Subscribe(ctx, messagequeue.SubjectGraphSearchResult, func(msgCtx context.Context, _ string, data []byte) error {
		var payload messagequeue.GraphSearchResultPayload
		if err := json.Unmarshal(data, &payload); err != nil {
			return fmt.Errorf("unmarshal graph search result: %w", err)
		}
		s.HandleSearchResult(msgCtx, &payload)
		return nil
	})
	if err != nil {
		cancelBuild()
		return nil, fmt.Errorf("subscribe graph search result: %w", err)
	}

	return []func(){cancelBuild, cancelSearch}, nil
}

// isUnhealthy returns true if the graph search worker recently failed.
func (s *GraphService) isUnhealthy() bool {
	s.healthMu.Lock()
	defer s.healthMu.Unlock()
	if s.lastSearchFailure.IsZero() {
		return false
	}
	return time.Since(s.lastSearchFailure) < healthCooldown
}

// recordFailure stores the current time as the last search failure.
func (s *GraphService) recordFailure() {
	s.healthMu.Lock()
	s.lastSearchFailure = time.Now()
	s.healthMu.Unlock()
}
