package llm

import "strings"

// SelectStrongestModel picks the best model from a list of discovered models.
// Ranking heuristic (in order of priority):
//  1. Output cost per token -- paid models correlate strongly with quality.
//  2. Model name pattern matching -- known model families have known strengths.
//  3. MaxTokens -- larger context windows as tiebreaker.
//
// Returns an empty string if the list is empty.
func SelectStrongestModel(models []DiscoveredModel) string {
	if len(models) == 0 {
		return ""
	}

	bestIdx := 0
	bestScore := modelScore(&models[0])
	for i := 1; i < len(models); i++ {
		if s := modelScore(&models[i]); s > bestScore {
			bestIdx = i
			bestScore = s
		}
	}
	return models[bestIdx].ModelName
}

// modelScore assigns a quality score to a discovered model.
func modelScore(m *DiscoveredModel) float64 {
	// Paid models: output cost is a strong quality signal.
	// More expensive models are almost always more capable.
	cost := max(0, m.OutputCostPer)
	if cost > 0 {
		// Scale so typical prices produce scores in 50-100 range.
		// GPT-4o: ~$15/1M = 1.5e-5 -> score ~75
		// Claude Opus: ~$75/1M = 7.5e-5 -> score ~100+
		// GPT-4o-mini: ~$0.6/1M = 6e-7 -> score ~30
		return cost * 5e6
	}

	// Free/local models: use name-based heuristics.
	// Check specific patterns first (longer matches) to avoid false hits.
	name := strings.ToLower(m.ModelName)
	patterns := []struct {
		substr string
		score  float64
	}{
		// Frontier models (if free tier / Groq-hosted)
		{"llama-4-maverick", 80},
		{"llama-4-scout", 75},
		{"llama-3.3-70b", 70},
		{"llama-3.1-70b", 60},
		{"mixtral-8x22b", 58},
		{"mixtral-8x7b", 45},
		{"llama-3.1-8b", 30},
		{"llama-3-8b", 28},
		{"gemma-2-27b", 50},
		{"gemma-2-9b", 35},
		{"phi-3", 32},
		{"qwen-2.5-72b", 65},
		{"qwen-2.5-32b", 55},
		{"deepseek-v3", 72},
		{"deepseek-r1", 74},
		{"deepseek-coder", 50},
		// Known premium model families
		{"claude-3-opus", 95},
		{"claude-3.5-sonnet", 93},
		{"claude-3-sonnet", 85},
		{"claude-3-haiku", 65},
		{"gpt-4o-mini", 63},
		{"gpt-4o", 90},
		{"gpt-4-turbo", 88},
		{"gpt-4", 82},
		{"gpt-3.5", 25},
		{"o1-preview", 92},
		{"o1-mini", 70},
		{"o3-mini", 78},
		{"gemini-1.5-pro", 88},
		{"gemini-2.0-flash", 76},
		{"gemini-1.5-flash", 60},
	}

	for _, p := range patterns {
		if strings.Contains(name, p.substr) {
			return p.score
		}
	}

	// Unknown model: use MaxTokens as rough proxy (larger context = newer model).
	if m.MaxTokens > 0 {
		return float64(m.MaxTokens) / 1e5 // 128k -> 1.28, not great but non-zero
	}

	return 1.0
}
