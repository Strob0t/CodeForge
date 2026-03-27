package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/Strob0t/CodeForge/internal/config"
	cfcontext "github.com/Strob0t/CodeForge/internal/domain/context"
	"github.com/Strob0t/CodeForge/internal/domain/knowledgebase"
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

// fetchRetrievalEntriesWithHits tries the sub-agent first (if enabled), falls back to single-shot search.
// so they can be used as seed symbols for graph search.
func (s *ContextOptimizerService) fetchRetrievalEntriesWithHits(ctx context.Context, projectID, prompt string) ([]cfcontext.ContextEntry, []messagequeue.RetrievalSearchHitPayload) {
	// Try sub-agent search first if enabled.
	if s.orchCfg.SubAgentEnabled {
		// Look up project-specific expansion prompt from config.
		var expansionPrompt string
		if proj, projErr := s.store.GetProject(ctx, projectID); projErr == nil && proj.Config != nil {
			expansionPrompt = proj.Config["expansion_prompt"]
		}
		subAgentModel := s.orchCfg.SubAgentModel
		if subAgentModel == "" && s.modelRegistry != nil {
			subAgentModel = s.modelRegistry.BestModel()
		}
		subResult, err := s.retrieval.SubAgentSearchSync(
			ctx, projectID, prompt,
			s.orchCfg.RetrievalTopK,
			s.orchCfg.SubAgentMaxQueries,
			subAgentModel,
			s.orchCfg.SubAgentRerank,
			expansionPrompt,
		)
		if err == nil {
			slog.Info("retrieval via sub-agent", "project_id", projectID, "hits", len(subResult.Results), "queries", len(subResult.ExpandedQueries))
			return hitsToEntries(subResult.Results), subResult.Results
		}
		slog.Warn("sub-agent search failed, falling back to single-shot", "project_id", projectID, "error", err)
	}

	// Fallback (or only path when sub-agent is disabled): single-shot search.
	result, err := s.retrieval.SearchSync(ctx, projectID, prompt, s.orchCfg.RetrievalTopK,
		s.orchCfg.RetrievalBM25Weight, s.orchCfg.RetrievalSemanticWeight)
	if err != nil {
		slog.Warn("retrieval search failed during context build", "project_id", projectID, "error", err)
		return nil, nil
	}

	return hitsToEntries(result.Results), result.Results
}

// hitsToEntries converts retrieval search hits to context entries.
// Uses percentile-based priority normalization (#5): RRF scores are typically 0.001–0.033,
// so raw int(score*100) yields 0–3. Instead, we normalize to the 60–85 range.
func hitsToEntries(hits []messagequeue.RetrievalSearchHitPayload) []cfcontext.ContextEntry {
	if len(hits) == 0 {
		return nil
	}

	// Find min/max scores for normalization.
	minScore, maxScore := hits[0].Score, hits[0].Score
	for _, h := range hits[1:] {
		if h.Score < minScore {
			minScore = h.Score
		}
		if h.Score > maxScore {
			maxScore = h.Score
		}
	}

	const (
		priorityMin = 60
		priorityMax = 85
	)

	entries := make([]cfcontext.ContextEntry, 0, len(hits))
	for _, hit := range hits {
		// Normalize score to priority range.
		priority := priorityMin
		if maxScore > minScore {
			normalized := (hit.Score - minScore) / (maxScore - minScore) // 0.0 to 1.0
			priority = priorityMin + int(normalized*float64(priorityMax-priorityMin))
		} else if len(hits) == 1 {
			// Single result gets midpoint priority.
			priority = (priorityMin + priorityMax) / 2
		}

		entries = append(entries, cfcontext.ContextEntry{
			Kind:     cfcontext.EntryHybrid,
			Path:     hit.Filepath,
			Content:  hit.Content,
			Tokens:   cfcontext.EstimateTokens(hit.Content),
			Priority: priority,
		})
	}
	return entries
}

// fetchGraphEntries performs a graph search using seed symbols from retrieval hits.
func (s *ContextOptimizerService) fetchGraphEntries(ctx context.Context, projectID, prompt string, retrievalHits []messagequeue.RetrievalSearchHitPayload) []cfcontext.ContextEntry {
	if s.graph == nil || !s.orchCfg.GraphEnabled {
		return nil
	}

	info := s.graph.GetStatus(projectID)
	if info == nil || info.Status != "ready" {
		return nil
	}

	// Extract seed symbols from retrieval hits.
	seen := make(map[string]bool)
	var seedSymbols []string
	for _, hit := range retrievalHits {
		if hit.SymbolName != "" && !seen[hit.SymbolName] {
			seen[hit.SymbolName] = true
			seedSymbols = append(seedSymbols, hit.SymbolName)
		}
	}

	// If no symbols from retrieval, try extracting keywords from the prompt.
	if len(seedSymbols) == 0 {
		keywords := extractKeywords(prompt)
		if len(keywords) > 5 {
			keywords = keywords[:5]
		}
		seedSymbols = keywords
	}

	if len(seedSymbols) == 0 {
		return nil
	}

	result, err := s.graph.SearchSync(ctx, projectID, seedSymbols, s.orchCfg.GraphMaxHops, s.orchCfg.GraphTopK)
	if err != nil {
		slog.Warn("graph search failed during context build", "project_id", projectID, "error", err)
		return nil
	}

	entries := make([]cfcontext.ContextEntry, 0, len(result.Results))
	for _, hit := range result.Results {
		// Priority: 70 - (distance * 10) -- hop 0 = 70, hop 1 = 60, hop 2 = 50
		priority := 70 - (hit.Distance * 10)
		if priority < 10 {
			priority = 10
		}

		content := fmt.Sprintf("[%s] %s (lines %d-%d)", hit.Kind, hit.SymbolName, hit.StartLine, hit.EndLine)
		entries = append(entries, cfcontext.ContextEntry{
			Kind:     cfcontext.EntryGraph,
			Path:     hit.Filepath,
			Content:  content,
			Tokens:   cfcontext.EstimateTokens(content),
			Priority: priority,
		})
	}

	slog.Info("graph context entries", "project_id", projectID, "hits", len(entries), "seeds", len(seedSymbols))
	return entries
}

// fetchKnowledgeBaseEntries searches knowledge bases attached to the project's scopes
// and returns matching content as context entries.
func (s *ContextOptimizerService) fetchKnowledgeBaseEntries(
	ctx context.Context,
	projectID, userMessage string,
) ([]cfcontext.ContextEntry, error) {
	scopes, err := s.store.GetScopesForProject(ctx, projectID)
	if err != nil || len(scopes) == 0 {
		return nil, err
	}

	var entries []cfcontext.ContextEntry
	seen := make(map[string]bool) // deduplicate KBs across scopes

	for i := range scopes {
		kbs, kbErr := s.store.ListKnowledgeBasesByScope(ctx, scopes[i].ID)
		if kbErr != nil || len(kbs) == 0 {
			continue
		}
		for j := range kbs {
			kb := &kbs[j]
			if seen[kb.ID] || kb.Status != "indexed" {
				continue
			}
			seen[kb.ID] = true

			kbEntries := s.processKnowledgeBase(ctx, kb, userMessage)
			entries = append(entries, kbEntries...)
		}
	}

	return entries, nil
}

// processKnowledgeBase fetches context entries from a single indexed knowledge base.
// It tries retrieval search first and falls back to reading the KB file directly.
func (s *ContextOptimizerService) processKnowledgeBase(
	ctx context.Context,
	kb *knowledgebase.KnowledgeBase,
	userMessage string,
) []cfcontext.ContextEntry {
	// Try retrieval search using the "kb:<id>" namespace.
	if s.retrieval != nil {
		kbProjectID := "kb:" + kb.ID
		if info := s.retrieval.GetIndexStatus(kbProjectID); info != nil && info.Status == "ready" {
			result, rErr := s.retrieval.SearchSync(ctx, kbProjectID, userMessage,
				3, s.orchCfg.RetrievalBM25Weight, s.orchCfg.RetrievalSemanticWeight)
			if rErr == nil && len(result.Results) > 0 {
				entries := make([]cfcontext.ContextEntry, 0, len(result.Results))
				for _, hit := range result.Results {
					entries = append(entries, cfcontext.ContextEntry{
						Kind:     cfcontext.EntryKnowledge,
						Path:     kb.Name,
						Content:  hit.Content,
						Tokens:   cfcontext.EstimateTokens(hit.Content),
						Priority: 75,
					})
				}
				return entries
			}
		}
	}

	// Fallback: read content directly from the KB file (truncated).
	if kb.ContentPath == "" {
		return nil
	}

	content, readErr := s.fs.ReadFile(ctx, kb.ContentPath)
	if readErr != nil || len(content) == 0 {
		return nil
	}

	text := string(content)
	const maxKBTokens = 2048
	tokens := cfcontext.EstimateTokens(text)
	if tokens > maxKBTokens {
		// Truncate to roughly maxKBTokens worth of characters.
		maxChars := maxKBTokens * 4
		if maxChars < len(text) {
			text = text[:maxChars]
		}
		tokens = maxKBTokens
	}

	return []cfcontext.ContextEntry{{
		Kind:     cfcontext.EntryKnowledge,
		Path:     kb.Name,
		Content:  text,
		Tokens:   tokens,
		Priority: 75,
	}}
}

// ---------------------------------------------------------------------------
// LLM context re-ranking (Phase 3 — Context Intelligence)
// ---------------------------------------------------------------------------

// contextEntriesToRerankPayload converts domain entries to NATS rerank payloads.
func contextEntriesToRerankPayload(entries []cfcontext.ContextEntry) []messagequeue.ContextRerankEntryPayload {
	out := make([]messagequeue.ContextRerankEntryPayload, len(entries))
	for i, e := range entries {
		out[i] = messagequeue.ContextRerankEntryPayload{
			Path: e.Path, Kind: string(e.Kind), Content: e.Content,
			Priority: e.Priority, Tokens: e.Tokens,
		}
	}
	return out
}

// rerankPayloadToContextEntries converts NATS rerank payloads back to domain entries.
func rerankPayloadToContextEntries(payloads []messagequeue.ContextRerankEntryPayload) []cfcontext.ContextEntry {
	out := make([]cfcontext.ContextEntry, len(payloads))
	for i, p := range payloads {
		out[i] = cfcontext.ContextEntry{
			Kind: cfcontext.EntryKind(p.Kind), Path: p.Path, Content: p.Content,
			Priority: p.Priority, Tokens: p.Tokens,
		}
	}
	return out
}

// RerankSync sends context entries to the Python worker for LLM re-ranking
// and blocks until the result arrives or the timeout expires.
func (s *ContextOptimizerService) RerankSync(ctx context.Context, projectID, query string, entries []cfcontext.ContextEntry) ([]cfcontext.ContextEntry, error) {
	requestID := uuid.New().String()
	ch := s.rerankWaiter.register(requestID)
	defer s.rerankWaiter.unregister(requestID)

	payload := messagequeue.ContextRerankRequestPayload{
		RequestID: requestID,
		ProjectID: projectID,
		Query:     query,
		Entries:   contextEntriesToRerankPayload(entries),
		Model:     s.orchCfg.ContextRerankModel,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return entries, fmt.Errorf("marshal rerank request: %w", err)
	}
	if err := s.queue.Publish(ctx, messagequeue.SubjectContextRerankRequest, data); err != nil {
		return entries, fmt.Errorf("publish rerank request: %w", err)
	}

	tctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	select {
	case result := <-ch:
		if result.Error != "" {
			return entries, fmt.Errorf("rerank worker: %s", result.Error)
		}
		return rerankPayloadToContextEntries(result.Entries), nil
	case <-tctx.Done():
		return entries, tctx.Err()
	}
}

// HandleRerankResult delivers a rerank result to the waiting caller.
func (s *ContextOptimizerService) HandleRerankResult(_ context.Context, payload *messagequeue.ContextRerankResultPayload) {
	s.rerankWaiter.deliver(payload.RequestID, payload)
}

// StartSubscribers subscribes to NATS subjects for context optimizer results.
func (s *ContextOptimizerService) StartSubscribers(ctx context.Context) ([]func(), error) {
	if s.queue == nil {
		return nil, nil
	}
	cancelRerank, err := s.queue.Subscribe(ctx, messagequeue.SubjectContextRerankResult, func(msgCtx context.Context, _ string, data []byte) error {
		var payload messagequeue.ContextRerankResultPayload
		if err := json.Unmarshal(data, &payload); err != nil {
			return fmt.Errorf("unmarshal context rerank result: %w", err)
		}
		s.HandleRerankResult(msgCtx, &payload)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("subscribe context rerank result: %w", err)
	}
	return []func(){cancelRerank}, nil
}

// scanWorkspaceFiles reads workspace files and scores them against the task prompt.
func (s *ContextOptimizerService) scanWorkspaceFiles(ctx context.Context, workspacePath, taskPrompt string) []cfcontext.ContextEntry {
	maxFiles := s.limits.MaxFiles
	maxFileSize := int64(s.limits.MaxFileSize)

	entries, err := s.fs.ReadDir(ctx, workspacePath)
	if err != nil {
		slog.Warn("cannot read workspace", "path", workspacePath, "error", err)
		return nil
	}

	var result []cfcontext.ContextEntry
	fileCount := 0

	for _, e := range entries {
		if fileCount >= maxFiles {
			break
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}

		if e.IsDir() {
			// Scan one level deep.
			subPath := filepath.Join(workspacePath, name)
			subEntries, err := s.fs.ReadDir(ctx, subPath)
			if err != nil {
				continue
			}
			for _, se := range subEntries {
				if fileCount >= maxFiles {
					break
				}
				if se.IsDir() || strings.HasPrefix(se.Name(), ".") {
					continue
				}
				entry := s.readAndScore(ctx, filepath.Join(subPath, se.Name()), name+"/"+se.Name(), taskPrompt, maxFileSize)
				if entry != nil {
					result = append(result, *entry)
					fileCount++
				}
			}
		} else {
			entry := s.readAndScore(ctx, filepath.Join(workspacePath, name), name, taskPrompt, maxFileSize)
			if entry != nil {
				result = append(result, *entry)
				fileCount++
			}
		}
	}

	return result
}

// readAndScore reads a file and returns a ContextEntry with relevance scoring.
func (s *ContextOptimizerService) readAndScore(ctx context.Context, absPath, relPath, taskPrompt string, maxSize int64) *cfcontext.ContextEntry {
	info, err := s.fs.Stat(ctx, absPath)
	if err != nil || info.Size() > maxSize || info.Size() == 0 {
		return nil
	}

	content, err := s.fs.ReadFile(ctx, absPath)
	if err != nil {
		return nil
	}

	text := string(content)
	score := ScoreFileRelevance(taskPrompt, relPath, text)
	if score == 0 {
		return nil
	}

	tokens := cfcontext.EstimateTokens(text)
	return &cfcontext.ContextEntry{
		Kind:     cfcontext.EntryFile,
		Path:     relPath,
		Content:  text,
		Tokens:   tokens,
		Priority: score,
	}
}

// ScoreFileRelevance returns a relevance score (0-100) for a file relative to a task prompt.
// Uses BM25-inspired scoring (P1-3): term frequency, inverse document frequency approximation,
// and document length normalization for more accurate relevance ranking.
func ScoreFileRelevance(taskPrompt, filePath, fileContent string) int {
	keywords := extractKeywords(taskPrompt)
	if len(keywords) == 0 {
		return 0
	}

	doc := strings.ToLower(filePath + " " + fileContent)
	docWords := strings.Fields(doc)
	docLen := len(docWords)
	if docLen == 0 {
		return 0
	}

	// BM25 parameters
	const k1 = 1.5
	const b = 0.75
	const avgDocLen = 200.0 // assumed average document length in words

	// Count term frequencies in the document
	termFreq := make(map[string]int, len(keywords))
	for _, w := range docWords {
		termFreq[w]++
	}

	n := float64(len(keywords))
	var score float64
	for _, kw := range keywords {
		tf := float64(termFreq[kw])
		if tf == 0 {
			// Also check substring containment (e.g. "auth" in "authentication")
			for _, w := range docWords {
				if strings.Contains(w, kw) {
					tf++
				}
			}
		}
		if tf == 0 {
			continue
		}

		// IDF approximation: treat each keyword as appearing in ~half the "corpus"
		// idf = log((N - df + 0.5) / (df + 0.5) + 1)
		df := 1.0 // simplified: each term has df=1
		idf := math.Log((n-df+0.5)/(df+0.5) + 1)

		// BM25 score component
		numerator := tf * (k1 + 1)
		denominator := tf + k1*(1-b+b*(float64(docLen)/avgDocLen))
		score += idf * (numerator / denominator)
	}

	// Normalize to 0-100 range
	// Max possible score: all keywords match with high TF
	maxScore := n * math.Log((n-1+0.5)/(1+0.5)+1) * ((k1 + 1) / (1 + k1*(1-b+b*(float64(docLen)/avgDocLen))))
	if maxScore <= 0 {
		return 0
	}

	normalized := int((score / maxScore) * 100)
	if normalized > 100 {
		normalized = 100
	}
	if normalized < 0 {
		normalized = 0
	}
	return normalized
}

// stopWords contains common English stop words filtered out during keyword extraction.
// Declared at package level to avoid re-allocating the map on every call.
var stopWords = map[string]bool{
	"the": true, "a": true, "an": true, "is": true, "are": true, "was": true,
	"were": true, "be": true, "been": true, "being": true, "have": true,
	"has": true, "had": true, "do": true, "does": true, "did": true,
	"will": true, "would": true, "could": true, "should": true, "may": true,
	"might": true, "shall": true, "can": true, "to": true, "of": true,
	"in": true, "for": true, "on": true, "with": true, "at": true,
	"by": true, "from": true, "as": true, "into": true, "through": true,
	"and": true, "or": true, "but": true, "not": true, "no": true,
	"if": true, "then": true, "else": true, "when": true, "up": true,
	"out": true, "that": true, "this": true, "it": true, "its": true,
}

// extractKeywords splits text into lowercase words, filtering out short and common words.
func extractKeywords(text string) []string {
	words := strings.Fields(strings.ToLower(text))
	var keywords []string
	seen := make(map[string]bool)
	for _, w := range words {
		// Strip non-alphanumeric edges.
		w = strings.Trim(w, ".,;:!?\"'()[]{}#/*-+=>< ")
		if len(w) < 3 || stopWords[w] || seen[w] {
			continue
		}
		seen[w] = true
		keywords = append(keywords, w)
	}
	return keywords
}
