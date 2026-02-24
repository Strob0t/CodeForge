package service

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/Strob0t/CodeForge/internal/config"
	cfcontext "github.com/Strob0t/CodeForge/internal/domain/context"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// ContextOptimizerService builds context packs for tasks by scoring file relevance,
// trimming to token budgets, and injecting shared context from team collaboration.
type ContextOptimizerService struct {
	store     database.Store
	orchCfg   *config.Orchestrator
	retrieval *RetrievalService
	graph     *GraphService

	// Guard against redundant builds for the same task (#16).
	buildMu    sync.Mutex
	builtTasks map[string]bool
}

// NewContextOptimizerService creates a ContextOptimizerService.
func NewContextOptimizerService(store database.Store, orchCfg *config.Orchestrator) *ContextOptimizerService {
	return &ContextOptimizerService{
		store:      store,
		orchCfg:    orchCfg,
		builtTasks: make(map[string]bool),
	}
}

// SetRetrieval wires the retrieval service for hybrid search injection.
func (s *ContextOptimizerService) SetRetrieval(r *RetrievalService) {
	s.retrieval = r
}

// SetGraph wires the graph service for GraphRAG injection.
func (s *ContextOptimizerService) SetGraph(g *GraphService) {
	s.graph = g
}

// GetPackByTask returns the existing context pack for a task, if any.
func (s *ContextOptimizerService) GetPackByTask(ctx context.Context, taskID string) (*cfcontext.ContextPack, error) {
	return s.store.GetContextPackByTask(ctx, taskID)
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
		if proj.WorkspacePath != "" {
			scanCh <- scanResult{entries: s.scanWorkspaceFiles(proj.WorkspacePath, t.Prompt)}
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
			retrievalTimeout += searchTimeout
			retrievalCtx, cancel := context.WithTimeout(ctx, retrievalTimeout)
			defer cancel()

			entries, hits := s.fetchRetrievalEntriesWithHits(retrievalCtx, projectID, t.Prompt)
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
		graphCh <- graphResult{entries: s.fetchGraphEntries(ctx, projectID, t.Prompt, rr.hits)}
	}()

	gr := <-graphCh
	candidates = append(candidates, gr.entries...)

	// Inject repo map if available for this project.
	repoMap, err := s.store.GetRepoMap(ctx, projectID)
	if err == nil && repoMap.MapText != "" {
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
		sc, err := s.store.GetSharedContextByTeam(ctx, teamID)
		if err == nil && sc != nil {
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

	if len(candidates) == 0 {
		slog.Debug("no context candidates found", "task_id", taskID, "project_id", projectID)
		return nil, nil
	}

	// Sort by priority descending.
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Priority > candidates[j].Priority
	})

	// Pack entries within budget.
	var packed []cfcontext.ContextEntry
	tokensUsed := 0
	for i := range candidates {
		if tokensUsed+candidates[i].Tokens > available {
			continue
		}
		packed = append(packed, candidates[i])
		tokensUsed += candidates[i].Tokens
	}

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
		subResult, err := s.retrieval.SubAgentSearchSync(
			ctx, projectID, prompt,
			s.orchCfg.RetrievalTopK,
			s.orchCfg.SubAgentMaxQueries,
			s.orchCfg.SubAgentModel,
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

// scanWorkspaceFiles reads workspace files and scores them against the task prompt.
func (s *ContextOptimizerService) scanWorkspaceFiles(workspacePath, taskPrompt string) []cfcontext.ContextEntry {
	const maxFiles = 50
	const maxFileSize = 32 * 1024 // 32 KB per file

	entries, err := os.ReadDir(workspacePath)
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
			subEntries, err := os.ReadDir(subPath)
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
				entry := s.readAndScore(filepath.Join(subPath, se.Name()), name+"/"+se.Name(), taskPrompt, maxFileSize)
				if entry != nil {
					result = append(result, *entry)
					fileCount++
				}
			}
		} else {
			entry := s.readAndScore(filepath.Join(workspacePath, name), name, taskPrompt, maxFileSize)
			if entry != nil {
				result = append(result, *entry)
				fileCount++
			}
		}
	}

	return result
}

// readAndScore reads a file and returns a ContextEntry with relevance scoring.
func (s *ContextOptimizerService) readAndScore(absPath, relPath, taskPrompt string, maxSize int64) *cfcontext.ContextEntry {
	info, err := os.Stat(absPath)
	if err != nil || info.Size() > maxSize || info.Size() == 0 {
		return nil
	}

	content, err := os.ReadFile(absPath) //nolint:gosec // G304: path constructed from validated project root
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

// extractKeywords splits text into lowercase words, filtering out short and common words.
func extractKeywords(text string) []string {
	stopWords := map[string]bool{
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
