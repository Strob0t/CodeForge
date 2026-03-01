package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/Strob0t/CodeForge/internal/adapter/ws"
	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/port/broadcast"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/port/eventstore"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// defaultSubAgentSearchTimeout is the fallback when config value is zero.
const defaultSubAgentSearchTimeout = 60 * time.Second

// healthCooldown is the duration after a failure during which requests fast-fail.
const healthCooldown = 30 * time.Second

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

// KBStatusUpdater can update knowledge base indexing status.
type KBStatusUpdater interface {
	UpdateKnowledgeBaseStatus(ctx context.Context, id, status string, chunkCount int) error
}

// RetrievalService manages hybrid retrieval indexing and search.
type RetrievalService struct {
	store   database.Store
	queue   messagequeue.Queue
	hub     broadcast.Broadcaster
	orchCfg *config.Orchestrator
	limits  *config.Limits
	events  eventstore.Store

	mu      sync.RWMutex
	indexes map[string]*RetrievalIndexInfo

	searchWaiter   *syncWaiter[messagequeue.RetrievalSearchResultPayload]
	subAgentWaiter *syncWaiter[messagequeue.SubAgentSearchResultPayload]

	// Health tracking: fast-fail when the worker recently failed (#3).
	healthMu            sync.Mutex
	lastSearchFailure   time.Time
	lastSubAgentFailure time.Time

	kbUpdater KBStatusUpdater
}

// NewRetrievalService creates a RetrievalService.
func NewRetrievalService(store database.Store, queue messagequeue.Queue, hub broadcast.Broadcaster, orchCfg *config.Orchestrator, limits *config.Limits) *RetrievalService {
	return &RetrievalService{
		store:          store,
		queue:          queue,
		hub:            hub,
		orchCfg:        orchCfg,
		limits:         limits,
		indexes:        make(map[string]*RetrievalIndexInfo),
		searchWaiter:   newSyncWaiter[messagequeue.RetrievalSearchResultPayload]("search"),
		subAgentWaiter: newSyncWaiter[messagequeue.SubAgentSearchResultPayload]("subagent"),
	}
}

// SetEventStore injects the event store used for recording sub-agent LLM costs.
func (s *RetrievalService) SetEventStore(es eventstore.Store) {
	s.events = es
}

// SetKBUpdater injects the knowledge base status updater.
func (s *RetrievalService) SetKBUpdater(u KBStatusUpdater) {
	s.kbUpdater = u
}

// RequestIndex publishes a request for index building to the Python worker.
// When workspacePath is non-empty it is used directly (e.g. for knowledge bases);
// otherwise the workspace path is resolved from the project store.
func (s *RetrievalService) RequestIndex(ctx context.Context, projectID, workspacePath, embeddingModel string) error {
	if workspacePath == "" {
		proj, err := s.store.GetProject(ctx, projectID)
		if err != nil {
			return fmt.Errorf("get project: %w", err)
		}
		workspacePath = proj.WorkspacePath
	}

	if embeddingModel == "" {
		embeddingModel = s.orchCfg.DefaultEmbeddingModel
	}

	payload := messagequeue.RetrievalIndexRequestPayload{
		ProjectID:      projectID,
		WorkspacePath:  workspacePath,
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

	// Update knowledge base status when the project ID has a "kb:" prefix.
	if s.kbUpdater != nil && strings.HasPrefix(payload.ProjectID, "kb:") {
		kbID := strings.TrimPrefix(payload.ProjectID, "kb:")
		kbStatus := "indexed"
		if payload.Error != "" {
			kbStatus = "error"
		}
		if err := s.kbUpdater.UpdateKnowledgeBaseStatus(ctx, kbID, kbStatus, payload.ChunkCount); err != nil {
			slog.Error("failed to update knowledge base status", "kb_id", kbID, "error", err)
		}
	}

	return nil
}

// SearchSync sends a search request and waits synchronously for the result.
// scopeID is optional — set when the search originates from a scope fan-out (observability).
func (s *RetrievalService) SearchSync(ctx context.Context, projectID, query string, topK int, bm25Weight, semanticWeight float64, scopeID ...string) (*messagequeue.RetrievalSearchResultPayload, error) {
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
	if len(scopeID) > 0 {
		payload.ScopeID = scopeID[0]
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal retrieval search request: %w", err)
	}

	if err := s.queue.Publish(ctx, messagequeue.SubjectRetrievalSearchRequest, data); err != nil {
		return nil, fmt.Errorf("publish retrieval search request: %w", err)
	}

	// Wait for result with timeout.
	timeoutCtx, cancel := context.WithTimeout(ctx, s.limits.SearchTimeout)
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
// expansionPrompt is optional; when non-empty it overrides the default query expansion system prompt.
func (s *RetrievalService) SubAgentSearchSync(ctx context.Context, projectID, query string, topK, maxQueries int, model string, rerank bool, expansionPrompt ...string) (*messagequeue.SubAgentSearchResultPayload, error) {
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
	if len(expansionPrompt) > 0 {
		payload.ExpansionPrompt = expansionPrompt[0]
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

// HandleSubAgentSearchResult delivers a sub-agent search result to the waiting caller
// and records any reported LLM cost in the event store for cost aggregation.
func (s *RetrievalService) HandleSubAgentSearchResult(ctx context.Context, payload *messagequeue.SubAgentSearchResultPayload) {
	s.subAgentWaiter.deliver(payload.RequestID, payload)

	// Record sub-agent LLM cost when the event store is available and cost > 0.
	if s.events != nil && (payload.CostUSD > 0 || payload.TokensIn > 0 || payload.TokensOut > 0) {
		costPayload, err := json.Marshal(map[string]string{
			"query":      payload.Query,
			"request_id": payload.RequestID,
			"hits":       fmt.Sprintf("%d", len(payload.Results)),
		})
		if err != nil {
			slog.Error("failed to marshal subagent cost payload", "error", err)
			return
		}
		ev := event.AgentEvent{
			ProjectID: payload.ProjectID,
			Type:      event.TypeToolCallResultEv,
			Payload:   costPayload,
			RequestID: payload.RequestID,
			Version:   1,
			ToolName:  "retrieval",
			Model:     payload.Model,
			TokensIn:  payload.TokensIn,
			TokensOut: payload.TokensOut,
			CostUSD:   payload.CostUSD,
		}
		if err := s.events.Append(ctx, &ev); err != nil {
			slog.Error("failed to record subagent search cost", "project_id", payload.ProjectID, "error", err)
		}
	}
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
