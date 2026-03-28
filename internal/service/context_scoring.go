package service

import (
	"context"
	"log/slog"
	"math"
	"path/filepath"
	"strings"

	cfcontext "github.com/Strob0t/CodeForge/internal/domain/context"
)

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
