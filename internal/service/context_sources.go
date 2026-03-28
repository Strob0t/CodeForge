package service

import (
	"context"
	"fmt"
	"log/slog"

	cfcontext "github.com/Strob0t/CodeForge/internal/domain/context"
	"github.com/Strob0t/CodeForge/internal/domain/knowledgebase"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

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
