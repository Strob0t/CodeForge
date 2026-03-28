package service

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/Strob0t/CodeForge/internal/config"
	cfcontext "github.com/Strob0t/CodeForge/internal/domain/context"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/port/filesystem"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// ContextOptimizerService builds context packs for tasks by scoring file relevance,
// trimming to token budgets, and injecting shared context from team collaboration.
type ContextOptimizerService struct {
	store         database.Store
	fs            filesystem.Provider
	orchCfg       *config.Orchestrator
	limits        *config.Limits
	retrieval     *RetrievalService
	graph         *GraphService
	lsp           *LSPService
	goalSvc       *GoalDiscoveryService
	modelRegistry *ModelRegistry
	queue         messagequeue.Queue
	rerankWaiter  *syncWaiter[messagequeue.ContextRerankResultPayload]

	// Guard against redundant builds for the same task (#16).
	buildMu    sync.Mutex
	builtTasks map[string]bool
}

// NewContextOptimizerService creates a ContextOptimizerService.
func NewContextOptimizerService(store database.Store, fs filesystem.Provider, orchCfg *config.Orchestrator, limits *config.Limits) *ContextOptimizerService {
	return &ContextOptimizerService{
		store:        store,
		fs:           fs,
		orchCfg:      orchCfg,
		limits:       limits,
		rerankWaiter: newSyncWaiter[messagequeue.ContextRerankResultPayload]("context-rerank"),
		builtTasks:   make(map[string]bool),
	}
}

// SetRetrieval wires the retrieval service for hybrid search injection.
func (s *ContextOptimizerService) SetRetrieval(r *RetrievalService) {
	s.retrieval = r
}

// SetModelRegistry injects the model registry for dynamic model resolution.
func (s *ContextOptimizerService) SetModelRegistry(r *ModelRegistry) {
	s.modelRegistry = r
}

// SetGraph wires the graph service for GraphRAG injection.
func (s *ContextOptimizerService) SetGraph(g *GraphService) {
	s.graph = g
}

// SetLSP wires the LSP service for diagnostic injection.
func (s *ContextOptimizerService) SetLSP(l *LSPService) {
	s.lsp = l
}

// SetGoalService wires the goal discovery service for context pack injection.
func (s *ContextOptimizerService) SetGoalService(svc *GoalDiscoveryService) {
	s.goalSvc = svc
}

// SetQueue wires the message queue for NATS-based reranking.
func (s *ContextOptimizerService) SetQueue(q messagequeue.Queue) { s.queue = q }

// GetPackByTask returns the existing context pack for a task, if any.
func (s *ContextOptimizerService) GetPackByTask(ctx context.Context, taskID string) (*cfcontext.ContextPack, error) {
	return s.store.GetContextPackByTask(ctx, taskID)
}

// ConversationContextOpts configures context assembly for conversation flows.
type ConversationContextOpts struct {
	Budget        int
	PromptReserve int
}

// BuildConversationContext assembles context entries for a conversation message
// without persisting a context pack. Returns entries directly.
func (s *ContextOptimizerService) BuildConversationContext(
	ctx context.Context,
	projectID, userMessage, teamID string,
	opts ConversationContextOpts,
) ([]cfcontext.ContextEntry, error) {
	proj, err := s.store.GetProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}

	budget := opts.Budget
	if budget <= 0 {
		budget = 2048
	}
	reserve := opts.PromptReserve
	if reserve <= 0 {
		reserve = 512
	}

	entries, _ := s.assembleAndPack(ctx, proj.ID, proj.WorkspacePath, userMessage, teamID, budget, reserve)
	return entries, nil
}

// BuildContextPack creates a context pack for a task by:
// 1. Scanning workspace files and scoring by keyword relevance (parallel with retrieval)
// 2. Injecting shared context items (if teamID is provided)
// 3. Packing entries within the token budget
// 4. Persisting the pack in the store
func (s *ContextOptimizerService) BuildContextPack(ctx context.Context, taskID, projectID, teamID string) (*cfcontext.ContextPack, error) {
	// Check-before-build: skip if already built for this task (#16).
	s.buildMu.Lock()
	if s.builtTasks[taskID] {
		s.buildMu.Unlock()
		existing, err := s.store.GetContextPackByTask(ctx, taskID)
		if err == nil && existing != nil {
			slog.Debug("context pack already built, returning existing", "task_id", taskID)
			return existing, nil
		}
		// If the stored pack is gone, re-acquire lock and fall through to rebuild.
		s.buildMu.Lock()
	}
	s.builtTasks[taskID] = true
	s.buildMu.Unlock()

	proj, err := s.store.GetProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}

	t, err := s.store.GetTask(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("get task: %w", err)
	}

	budget := s.orchCfg.DefaultContextBudget
	if budget <= 0 {
		budget = 4096
	}
	reserve := s.orchCfg.PromptReserve
	if reserve <= 0 {
		reserve = 1024
	}

	packed, tokensUsed := s.assembleAndPack(ctx, proj.ID, proj.WorkspacePath, t.Prompt, teamID, budget, reserve)

	if len(packed) == 0 {
		return nil, nil
	}

	pack := &cfcontext.ContextPack{
		TaskID:      taskID,
		ProjectID:   projectID,
		TokenBudget: budget,
		TokensUsed:  tokensUsed,
		Entries:     packed,
	}

	if err := s.store.CreateContextPack(ctx, pack); err != nil {
		return nil, fmt.Errorf("persist context pack: %w", err)
	}

	slog.Info("context pack built",
		"task_id", taskID,
		"entries", len(packed),
		"tokens_used", tokensUsed,
		"budget", budget,
	)
	return pack, nil
}

// assembleAndPack runs the parallel context assembly pipeline and packs entries within
// the token budget. Shared by BuildContextPack and BuildConversationContext.
func (s *ContextOptimizerService) assembleAndPack(
	ctx context.Context,
	projectID, workspacePath, prompt, teamID string,
	budget, reserve int,
) (entries []cfcontext.ContextEntry, tokensUsed int) {
	available := budget - reserve
	if available <= 0 {
		available = budget / 2
	}

	var candidates []cfcontext.ContextEntry

	// Run workspace scan and retrieval in parallel (#15).
	type scanResult struct {
		entries []cfcontext.ContextEntry
	}
	type retrievalResult struct {
		entries []cfcontext.ContextEntry
		hits    []messagequeue.RetrievalSearchHitPayload
	}
	type graphResult struct {
		entries []cfcontext.ContextEntry
	}

	scanCh := make(chan scanResult, 1)
	retrievalCh := make(chan retrievalResult, 1)
	graphCh := make(chan graphResult, 1)

	// Workspace scan goroutine.
	go func() {
		if workspacePath != "" {
			scanCh <- scanResult{entries: s.scanWorkspaceFiles(ctx, workspacePath, prompt)}
		} else {
			scanCh <- scanResult{}
		}
	}()

	// Retrieval goroutine with shared deadline (#4).
	go func() {
		if s.retrieval == nil {
			retrievalCh <- retrievalResult{}
			return
		}
		if info := s.retrieval.GetIndexStatus(projectID); info != nil && info.Status == "ready" {
			// Create a shared deadline for the entire retrieval path (sub-agent + fallback).
			retrievalTimeout := s.orchCfg.SubAgentTimeout
			if retrievalTimeout <= 0 {
				retrievalTimeout = defaultSubAgentSearchTimeout
			}
			// Add headroom for the single-shot fallback.
			retrievalTimeout += s.limits.SearchTimeout
			retrievalCtx, cancel := context.WithTimeout(ctx, retrievalTimeout)
			defer cancel()

			entries, hits := s.fetchRetrievalEntriesWithHits(retrievalCtx, projectID, prompt)
			retrievalCh <- retrievalResult{entries: entries, hits: hits}
		} else {
			retrievalCh <- retrievalResult{}
		}
	}()

	// Collect results from scan and retrieval first (graph needs retrieval hits).
	sr := <-scanCh
	candidates = append(candidates, sr.entries...)

	rr := <-retrievalCh
	candidates = append(candidates, rr.entries...)

	// Graph goroutine — uses retrieval hits as seed symbols.
	go func() {
		graphCh <- graphResult{entries: s.fetchGraphEntries(ctx, projectID, prompt, rr.hits)}
	}()

	gr := <-graphCh
	candidates = append(candidates, gr.entries...)

	// Inject repo map if available for this project.
	repoMap, rmErr := s.store.GetRepoMap(ctx, projectID)
	if rmErr == nil && repoMap.MapText != "" {
		candidates = append(candidates, cfcontext.ContextEntry{
			Kind:     cfcontext.EntryRepoMap,
			Path:     "repo-map",
			Content:  repoMap.MapText,
			Tokens:   repoMap.TokenCount,
			Priority: 85,
		})
	}

	// Inject shared context if team is specified.
	if teamID != "" {
		sc, scErr := s.store.GetSharedContextByTeam(ctx, teamID)
		if scErr == nil && sc != nil {
			for _, item := range sc.Items {
				candidates = append(candidates, cfcontext.ContextEntry{
					Kind:     cfcontext.EntryShared,
					Path:     item.Key,
					Content:  item.Value,
					Tokens:   item.Tokens,
					Priority: 90, // Shared context is high priority.
				})
			}
		}
	}

	// LSP diagnostics injection (high priority — errors agents should know about).
	if s.lsp != nil {
		diagEntries := s.lsp.DiagnosticsAsContextEntries(projectID)
		candidates = append(candidates, diagEntries...)
	}

	// Goal entries injection (vision, requirements, constraints, state).
	if s.goalSvc != nil {
		goalEntries, gErr := s.goalSvc.AsContextEntries(ctx, projectID)
		if gErr == nil {
			candidates = append(candidates, goalEntries...)
		}
	}

	// Knowledge base entries (matched to project scopes).
	kbEntries, kbErr := s.fetchKnowledgeBaseEntries(ctx, projectID, prompt)
	if kbErr == nil && len(kbEntries) > 0 {
		candidates = append(candidates, kbEntries...)
		slog.Info("knowledge base entries added", "count", len(kbEntries))
	}

	if len(candidates) == 0 {
		return nil, 0
	}

	// Deduplicate near-duplicate candidates using SimHash (B2).
	// This removes overlapping content from different retrieval sources
	// (BM25, semantic search, GraphRAG) before packing into the token budget.
	candidates = deduplicateCandidates(candidates, defaultDedupThreshold)

	// LLM re-ranking (if enabled and queue is available).
	if s.orchCfg.ContextRerankEnabled && s.queue != nil && len(candidates) > 1 {
		reranked, err := s.RerankSync(ctx, projectID, prompt, candidates)
		if err != nil {
			slog.Warn("context rerank failed, using original order", "error", err)
		} else {
			candidates = reranked
		}
	}

	// Pack entries within budget (already sorted by priority descending from dedup).
	for i := range candidates {
		if tokensUsed+candidates[i].Tokens > available {
			continue
		}
		entries = append(entries, candidates[i])
		tokensUsed += candidates[i].Tokens
	}

	return entries, tokensUsed
}
