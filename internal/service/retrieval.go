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

// defaultSubAgentSearchTimeout is the fallback when config value is zero.
const defaultSubAgentSearchTimeout = 60 * time.Second

// healthCooldown is the duration after a failure during which requests fast-fail.
const healthCooldown = 30 * time.Second

// ---------------------------------------------------------------------------
// syncWaiter — generic correlation-ID-based waiter (#7 DRY)
// ---------------------------------------------------------------------------

// syncWaiter manages a set of channel-based waiters keyed by correlation ID.
type syncWaiter[T any] struct {
	mu      sync.Mutex
	waiters map[string]chan *T
	label   string // for logging
}

func newSyncWaiter[T any](label string) *syncWaiter[T] {
	return &syncWaiter[T]{
		waiters: make(map[string]chan *T),
		label:   label,
	}
}

// register creates a buffered channel for the given request ID.
func (w *syncWaiter[T]) register(requestID string) chan *T {
	ch := make(chan *T, 1)
	w.mu.Lock()
	w.waiters[requestID] = ch
	w.mu.Unlock()
	return ch
}

// unregister removes the waiter for the given request ID.
func (w *syncWaiter[T]) unregister(requestID string) {
	w.mu.Lock()
	delete(w.waiters, requestID)
	w.mu.Unlock()
}

// deliver sends a result to the waiting channel and removes the waiter.
// Returns false if no waiter was registered for the given ID.
func (w *syncWaiter[T]) deliver(requestID string, payload *T) bool {
	w.mu.Lock()
	ch, ok := w.waiters[requestID]
	if ok {
		delete(w.waiters, requestID)
	}
	w.mu.Unlock()

	if !ok {
		slog.Warn("no waiter for "+w.label+" result", "request_id", requestID)
		return false
	}

	ch <- payload
	return true
}

// ---------------------------------------------------------------------------
// RetrievalService
// ---------------------------------------------------------------------------

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

	searchWaiter   *syncWaiter[messagequeue.RetrievalSearchResultPayload]
	subAgentWaiter *syncWaiter[messagequeue.SubAgentSearchResultPayload]

	// Health tracking: fast-fail when the worker recently failed (#3).
	healthMu            sync.Mutex
	lastSearchFailure   time.Time
	lastSubAgentFailure time.Time
}

// NewRetrievalService creates a RetrievalService.
func NewRetrievalService(store database.Store, queue messagequeue.Queue, hub broadcast.Broadcaster, orchCfg *config.Orchestrator) *RetrievalService {
	return &RetrievalService{
		store:          store,
		queue:          queue,
		hub:            hub,
		orchCfg:        orchCfg,
		indexes:        make(map[string]*RetrievalIndexInfo),
		searchWaiter:   newSyncWaiter[messagequeue.RetrievalSearchResultPayload]("search"),
		subAgentWaiter: newSyncWaiter[messagequeue.SubAgentSearchResultPayload]("subagent"),
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
	// Fast-fail if the worker recently failed (#3).
	if s.isUnhealthy(&s.lastSearchFailure) {
		return nil, fmt.Errorf("search skipped: worker recently unhealthy (project %s)", projectID)
	}

	requestID, err := generateRequestID()
	if err != nil {
		return nil, err
	}

	ch := s.searchWaiter.register(requestID)
	defer s.searchWaiter.unregister(requestID)

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
			s.recordFailure(&s.lastSearchFailure)
			return nil, fmt.Errorf("search error: %s", result.Error)
		}
		return result, nil
	case <-timeoutCtx.Done():
		s.recordFailure(&s.lastSearchFailure)
		return nil, fmt.Errorf("search timeout for project %s", projectID)
	}
}

// HandleSearchResult delivers a search result to the waiting caller.
func (s *RetrievalService) HandleSearchResult(_ context.Context, payload *messagequeue.RetrievalSearchResultPayload) {
	s.searchWaiter.deliver(payload.RequestID, payload)
}

// SubAgentSearchSync sends a sub-agent search request and waits synchronously for the result.
func (s *RetrievalService) SubAgentSearchSync(ctx context.Context, projectID, query string, topK, maxQueries int, model string, rerank bool) (*messagequeue.SubAgentSearchResultPayload, error) {
	// Fast-fail if the worker recently failed (#3).
	if s.isUnhealthy(&s.lastSubAgentFailure) {
		return nil, fmt.Errorf("subagent search skipped: worker recently unhealthy (project %s)", projectID)
	}

	requestID, err := generateRequestID()
	if err != nil {
		return nil, err
	}

	ch := s.subAgentWaiter.register(requestID)
	defer s.subAgentWaiter.unregister(requestID)

	// Publish sub-agent search request.
	payload := messagequeue.SubAgentSearchRequestPayload{
		ProjectID:  projectID,
		Query:      query,
		RequestID:  requestID,
		TopK:       topK,
		MaxQueries: maxQueries,
		Model:      model,
		Rerank:     rerank,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal subagent search request: %w", err)
	}

	if err := s.queue.Publish(ctx, messagequeue.SubjectSubAgentSearchRequest, data); err != nil {
		return nil, fmt.Errorf("publish subagent search request: %w", err)
	}

	// Wait for result with timeout.
	timeout := s.orchCfg.SubAgentTimeout
	if timeout <= 0 {
		timeout = defaultSubAgentSearchTimeout
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	select {
	case result := <-ch:
		if result.Error != "" {
			s.recordFailure(&s.lastSubAgentFailure)
			return nil, fmt.Errorf("subagent search error: %s", result.Error)
		}
		return result, nil
	case <-timeoutCtx.Done():
		s.recordFailure(&s.lastSubAgentFailure)
		return nil, fmt.Errorf("subagent search timeout for project %s", projectID)
	}
}

// HandleSubAgentSearchResult delivers a sub-agent search result to the waiting caller.
func (s *RetrievalService) HandleSubAgentSearchResult(_ context.Context, payload *messagequeue.SubAgentSearchResultPayload) {
	s.subAgentWaiter.deliver(payload.RequestID, payload)
}

// SubAgentDefaults returns the orchestrator's sub-agent configuration defaults.
func (s *RetrievalService) SubAgentDefaults() (model string, maxQueries int, rerank bool) {
	return s.orchCfg.SubAgentModel, s.orchCfg.SubAgentMaxQueries, s.orchCfg.SubAgentRerank
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

	cancelSubAgent, err := s.queue.Subscribe(ctx, messagequeue.SubjectSubAgentSearchResult, func(msgCtx context.Context, _ string, data []byte) error {
		var payload messagequeue.SubAgentSearchResultPayload
		if err := json.Unmarshal(data, &payload); err != nil {
			return fmt.Errorf("unmarshal subagent search result: %w", err)
		}
		s.HandleSubAgentSearchResult(msgCtx, &payload)
		return nil
	})
	if err != nil {
		cancelIndex()
		cancelSearch()
		return nil, fmt.Errorf("subscribe subagent search result: %w", err)
	}

	return []func(){cancelIndex, cancelSearch, cancelSubAgent}, nil
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// generateRequestID creates a random 16-byte hex correlation ID.
func generateRequestID() (string, error) {
	idBytes := make([]byte, 16)
	if _, err := rand.Read(idBytes); err != nil {
		return "", fmt.Errorf("generate request id: %w", err)
	}
	return hex.EncodeToString(idBytes), nil
}

// isUnhealthy returns true if the given failure timestamp is within the health cooldown.
func (s *RetrievalService) isUnhealthy(lastFailure *time.Time) bool {
	s.healthMu.Lock()
	defer s.healthMu.Unlock()
	if lastFailure.IsZero() {
		return false
	}
	return time.Since(*lastFailure) < healthCooldown
}

// recordFailure stores the current time as the last failure for the given path.
func (s *RetrievalService) recordFailure(lastFailure *time.Time) {
	s.healthMu.Lock()
	*lastFailure = time.Now()
	s.healthMu.Unlock()
}
