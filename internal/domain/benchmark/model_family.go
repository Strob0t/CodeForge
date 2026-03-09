package benchmark

import "strings"

// ModelFamily extracts the provider family from a model identifier.
// "openai/gpt-4o" -> "openai", "anthropic/claude-3" -> "anthropic",
// "ollama/llama3" -> "local", "gpt-4" -> "openai".
func ModelFamily(model string) string {
	if model == "" {
		return "unknown"
	}

	// If the model string contains a slash, the prefix is the provider.
	if idx := strings.Index(model, "/"); idx > 0 {
		prefix := strings.ToLower(model[:idx])
		switch prefix {
		case "ollama", "lm-studio":
			return "local"
		default:
			return prefix
		}
	}

	// Heuristic matching for unqualified model names.
	lower := strings.ToLower(model)
	switch {
	case strings.HasPrefix(lower, "gpt-") || strings.HasPrefix(lower, "o1") || strings.HasPrefix(lower, "o3"):
		return "openai"
	case strings.HasPrefix(lower, "claude"):
		return "anthropic"
	case strings.HasPrefix(lower, "gemini") || strings.HasPrefix(lower, "gemma"):
		return "google"
	case strings.HasPrefix(lower, "llama"):
		return "meta-llama"
	case strings.HasPrefix(lower, "mistral") || strings.HasPrefix(lower, "mixtral"):
		return "mistral"
	case strings.HasPrefix(lower, "deepseek"):
		return "deepseek"
	case strings.HasPrefix(lower, "qwen"):
		return "qwen"
	default:
		return "unknown"
	}
}
