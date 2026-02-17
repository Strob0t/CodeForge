package service

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Strob0t/CodeForge/internal/config"
	cfcontext "github.com/Strob0t/CodeForge/internal/domain/context"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

// ContextOptimizerService builds context packs for tasks by scoring file relevance,
// trimming to token budgets, and injecting shared context from team collaboration.
type ContextOptimizerService struct {
	store     database.Store
	orchCfg   *config.Orchestrator
	retrieval *RetrievalService
}

// NewContextOptimizerService creates a ContextOptimizerService.
func NewContextOptimizerService(store database.Store, orchCfg *config.Orchestrator) *ContextOptimizerService {
	return &ContextOptimizerService{store: store, orchCfg: orchCfg}
}

// SetRetrieval wires the retrieval service for hybrid search injection.
func (s *ContextOptimizerService) SetRetrieval(r *RetrievalService) {
	s.retrieval = r
}

// GetPackByTask returns the existing context pack for a task, if any.
func (s *ContextOptimizerService) GetPackByTask(ctx context.Context, taskID string) (*cfcontext.ContextPack, error) {
	return s.store.GetContextPackByTask(ctx, taskID)
}

// BuildContextPack creates a context pack for a task by:
// 1. Scanning workspace files and scoring by keyword relevance
// 2. Injecting shared context items (if teamID is provided)
// 3. Packing entries within the token budget
// 4. Persisting the pack in the store
func (s *ContextOptimizerService) BuildContextPack(ctx context.Context, taskID, projectID, teamID string) (*cfcontext.ContextPack, error) {
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

	// Scan workspace files if workspace path is set.
	if proj.WorkspacePath != "" {
		fileEntries := s.scanWorkspaceFiles(proj.WorkspacePath, t.Prompt)
		candidates = append(candidates, fileEntries...)
	}

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

	// Inject hybrid retrieval results if index is ready.
	if s.retrieval != nil {
		if info := s.retrieval.GetIndexStatus(projectID); info != nil && info.Status == "ready" {
			result, err := s.retrieval.SearchSync(ctx, projectID, t.Prompt, 10,
				s.orchCfg.RetrievalBM25Weight, s.orchCfg.RetrievalSemanticWeight)
			if err != nil {
				slog.Warn("retrieval search failed during context build", "project_id", projectID, "error", err)
			} else {
				for _, hit := range result.Results {
					candidates = append(candidates, cfcontext.ContextEntry{
						Kind:     cfcontext.EntryHybrid,
						Path:     hit.Filepath,
						Content:  hit.Content,
						Tokens:   cfcontext.EstimateTokens(hit.Content),
						Priority: int(hit.Score * 100),
					})
				}
			}
		}
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

	content, err := os.ReadFile(absPath)
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
// It uses simple keyword matching: counts how many significant words from the prompt
// appear in either the file path or file content.
func ScoreFileRelevance(taskPrompt, filePath, fileContent string) int {
	words := extractKeywords(taskPrompt)
	if len(words) == 0 {
		return 0
	}

	target := strings.ToLower(filePath + " " + fileContent)
	matches := 0
	for _, w := range words {
		if strings.Contains(target, w) {
			matches++
		}
	}

	score := (matches * 100) / len(words)
	if score > 100 {
		score = 100
	}
	return score
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
