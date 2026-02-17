package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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

const searchTimeout = 30 * time.Second

// RetrievalIndexInfo holds the in-memory state of a project's retrieval index.
type RetrievalIndexInfo struct {
	ProjectID      string
	Status         string // "building", "ready", "error"
	FileCount      int
	ChunkCount     int
	EmbeddingModel string
	Error          string
}

// RetrievalService manages hybrid retrieval indexing and search.
type RetrievalService struct {
	store   database.Store
	queue   messagequeue.Queue
	hub     broadcast.Broadcaster
	orchCfg *config.Orchestrator

	mu      sync.RWMutex
	indexes map[string]*RetrievalIndexInfo

	searchMu      sync.Mutex
	searchWaiters map[string]chan *messagequeue.RetrievalSearchResultPayload
}

// NewRetrievalService creates a RetrievalService.
func NewRetrievalService(store database.Store, queue messagequeue.Queue, hub broadcast.Broadcaster, orchCfg *config.Orchestrator) *RetrievalService {
	return &RetrievalService{
		store:         store,
		queue:         queue,
		hub:           hub,
		orchCfg:       orchCfg,
		indexes:       make(map[string]*RetrievalIndexInfo),
		searchWaiters: make(map[string]chan *messagequeue.RetrievalSearchResultPayload),
	}
}

// RequestIndex publishes a request for index building to the Python worker.
func (s *RetrievalService) RequestIndex(ctx context.Context, projectID, embeddingModel string) error {
	proj, err := s.store.GetProject(ctx, projectID)
	if err != nil {
		return fmt.Errorf("get project: %w", err)
	}

	if embeddingModel == "" {
		embeddingModel = s.orchCfg.DefaultEmbeddingModel
	}

	payload := messagequeue.RetrievalIndexRequestPayload{
		ProjectID:      projectID,
		WorkspacePath:  proj.WorkspacePath,
		EmbeddingModel: embeddingModel,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal retrieval index request: %w", err)
	}

	if err := s.queue.Publish(ctx, messagequeue.SubjectRetrievalIndexRequest, data); err != nil {
		return fmt.Errorf("publish retrieval index request: %w", err)
	}

	s.mu.Lock()
	s.indexes[projectID] = &RetrievalIndexInfo{
		ProjectID:      projectID,
		Status:         "building",
		EmbeddingModel: embeddingModel,
	}
	s.mu.Unlock()

	s.hub.BroadcastEvent(ctx, ws.EventRetrievalStatus, ws.RetrievalStatusEvent{
		ProjectID:      projectID,
		Status:         "building",
		EmbeddingModel: embeddingModel,
	})

	slog.Info("retrieval index requested", "project_id", projectID, "model", embeddingModel)
	return nil
}

// HandleIndexResult processes the result of an index build from the Python worker.
func (s *RetrievalService) HandleIndexResult(ctx context.Context, payload *messagequeue.RetrievalIndexResultPayload) error {
	status := payload.Status
	if payload.Error != "" {
		status = "error"
	}

	s.mu.Lock()
	s.indexes[payload.ProjectID] = &RetrievalIndexInfo{
		ProjectID:      payload.ProjectID,
		Status:         status,
		FileCount:      payload.FileCount,
		ChunkCount:     payload.ChunkCount,
		EmbeddingModel: payload.EmbeddingModel,
		Error:          payload.Error,
	}
	s.mu.Unlock()

	s.hub.BroadcastEvent(ctx, ws.EventRetrievalStatus, ws.RetrievalStatusEvent{
		ProjectID:      payload.ProjectID,
		Status:         status,
		FileCount:      payload.FileCount,
		ChunkCount:     payload.ChunkCount,
		EmbeddingModel: payload.EmbeddingModel,
		Error:          payload.Error,
	})

	if payload.Error != "" {
		slog.Error("retrieval index failed", "project_id", payload.ProjectID, "error", payload.Error)
	} else {
		slog.Info("retrieval index ready",
			"project_id", payload.ProjectID,
			"files", payload.FileCount,
			"chunks", payload.ChunkCount,
		)
	}
	return nil
}

// SearchSync sends a search request and waits synchronously for the result.
func (s *RetrievalService) SearchSync(ctx context.Context, projectID, query string, topK int, bm25Weight, semanticWeight float64) (*messagequeue.RetrievalSearchResultPayload, error) {
	// Generate correlation ID.
	idBytes := make([]byte, 16)
	if _, err := rand.Read(idBytes); err != nil {
		return nil, fmt.Errorf("generate request id: %w", err)
	}
	requestID := hex.EncodeToString(idBytes)

	// Register waiter.
	ch := make(chan *messagequeue.RetrievalSearchResultPayload, 1)
	s.searchMu.Lock()
	s.searchWaiters[requestID] = ch
	s.searchMu.Unlock()

	defer func() {
		s.searchMu.Lock()
		delete(s.searchWaiters, requestID)
		s.searchMu.Unlock()
	}()

	// Publish search request.
	payload := messagequeue.RetrievalSearchRequestPayload{
		ProjectID:      projectID,
		Query:          query,
		RequestID:      requestID,
		TopK:           topK,
		BM25Weight:     bm25Weight,
		SemanticWeight: semanticWeight,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal retrieval search request: %w", err)
	}

	if err := s.queue.Publish(ctx, messagequeue.SubjectRetrievalSearchRequest, data); err != nil {
		return nil, fmt.Errorf("publish retrieval search request: %w", err)
	}

	// Wait for result with timeout.
	timeoutCtx, cancel := context.WithTimeout(ctx, searchTimeout)
	defer cancel()

	select {
	case result := <-ch:
		if result.Error != "" {
			return nil, fmt.Errorf("search error: %s", result.Error)
		}
		return result, nil
	case <-timeoutCtx.Done():
		return nil, fmt.Errorf("search timeout for project %s", projectID)
	}
}

// HandleSearchResult delivers a search result to the waiting caller.
func (s *RetrievalService) HandleSearchResult(_ context.Context, payload *messagequeue.RetrievalSearchResultPayload) {
	s.searchMu.Lock()
	ch, ok := s.searchWaiters[payload.RequestID]
	if ok {
		delete(s.searchWaiters, payload.RequestID)
	}
	s.searchMu.Unlock()

	if !ok {
		slog.Warn("no waiter for search result", "request_id", payload.RequestID)
		return
	}

	ch <- payload
}

// GetIndexStatus returns the in-memory index info for a project, or nil if unknown.
func (s *RetrievalService) GetIndexStatus(projectID string) *RetrievalIndexInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.indexes[projectID]
}

// StartSubscribers subscribes to retrieval result subjects and returns cancel funcs.
func (s *RetrievalService) StartSubscribers(ctx context.Context) ([]func(), error) {
	cancelIndex, err := s.queue.Subscribe(ctx, messagequeue.SubjectRetrievalIndexResult, func(msgCtx context.Context, _ string, data []byte) error {
		var payload messagequeue.RetrievalIndexResultPayload
		if err := json.Unmarshal(data, &payload); err != nil {
			return fmt.Errorf("unmarshal retrieval index result: %w", err)
		}
		return s.HandleIndexResult(msgCtx, &payload)
	})
	if err != nil {
		return nil, fmt.Errorf("subscribe retrieval index result: %w", err)
	}

	cancelSearch, err := s.queue.Subscribe(ctx, messagequeue.SubjectRetrievalSearchResult, func(msgCtx context.Context, _ string, data []byte) error {
		var payload messagequeue.RetrievalSearchResultPayload
		if err := json.Unmarshal(data, &payload); err != nil {
			return fmt.Errorf("unmarshal retrieval search result: %w", err)
		}
		s.HandleSearchResult(msgCtx, &payload)
		return nil
	})
	if err != nil {
		cancelIndex()
		return nil, fmt.Errorf("subscribe retrieval search result: %w", err)
	}

	return []func(){cancelIndex, cancelSearch}, nil
}
