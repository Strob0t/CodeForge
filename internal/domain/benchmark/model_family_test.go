package benchmark

import "testing"

func TestModelFamily(t *testing.T) {
	tests := []struct {
		model    string
		expected string
	}{
		// Provider-prefixed names.
		{"anthropic/claude-sonnet-4", "anthropic"},
		{"openai/gpt-4o", "openai"},
		{"mistral/mistral-large-latest", "mistral"},
		{"meta-llama/llama-3.1-70b", "meta-llama"},
		{"google/gemini-2.0-flash", "google"},
		{"ollama/llama3", "local"},
		{"lm-studio/model", "local"},

		// Bare model names (no prefix).
		{"claude-sonnet-4", "anthropic"},
		{"gpt-4o", "openai"},
		{"o1-preview", "openai"},
		{"gemini-pro", "google"},
		{"mistral-large-latest", "mistral"},
		{"mixtral-8x7b", "mistral"},
		{"llama-3.1-70b", "meta-llama"},

		// Unknown and edge cases.
		{"unknown-model", "unknown"},
		{"", "unknown"},

		// Custom provider prefix falls through to default.
		{"deepseek/deepseek-coder", "deepseek"},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			if got := ModelFamily(tt.model); got != tt.expected {
				t.Errorf("ModelFamily(%q) = %q, want %q", tt.model, got, tt.expected)
			}
		})
	}
}
