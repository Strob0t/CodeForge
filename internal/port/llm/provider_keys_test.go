package llm_test

import (
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/port/llm"
)

func noEnv(string) string { return "" }

func envOf(vars map[string]string) func(string) string {
	return func(key string) string { return vars[key] }
}

// Which providers' models can be called with the installation's own key
// (KI-125): the Core lists their models and may pick one as default.
func TestProviderKeys_HasKey(t *testing.T) {
	tests := []struct {
		name     string
		named    []string
		env      map[string]string
		provider string
		want     bool
	}{
		{"cloud provider without a key", nil, nil, "anthropic", false},
		{"named by the operator (compose derives the names)", []string{"anthropic"}, nil, "anthropic", true},
		{"another provider is named", []string{"openai"}, nil, "anthropic", false},
		{"key variable in this process' environment", nil, map[string]string{"GROQ_API_KEY": "gsk-1"}, "groq", true},
		{"whitespace-only key is no key", nil, map[string]string{"GROQ_API_KEY": "  "}, "groq", false},
		{"catalogue provider without a key", nil, nil, "openrouter", false},
		{"OpenAI-compatible service without a key", nil, nil, "chutes", false},
		{"local server needs no key", nil, nil, "ollama", true},
		{"LM Studio needs no key", nil, nil, "lm_studio", true},
		{"unknown provider is assumed keyed, as in the worker", nil, nil, "newprovider", true},
		{"model without a provider prefix", nil, nil, "", true},
		{"names are trimmed and lower-cased", []string{" Anthropic "}, nil, "anthropic", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keys, err := llm.NewProviderKeys(tt.named, envOf(tt.env))
			if err != nil {
				t.Fatalf("NewProviderKeys: %v", err)
			}
			if got := keys.HasKey(tt.provider); got != tt.want {
				t.Errorf("HasKey(%q) = %v, want %v", tt.provider, got, tt.want)
			}
		})
	}
}

func TestProviderKeys_Names(t *testing.T) {
	keys, err := llm.NewProviderKeys([]string{"openai", "anthropic", "ollama"}, envOf(map[string]string{"GROQ_API_KEY": "gsk-1"}))
	if err != nil {
		t.Fatalf("NewProviderKeys: %v", err)
	}
	if got, want := strings.Join(keys.Names(), ","), "anthropic,groq,openai"; got != want {
		t.Errorf("Names() = %q, want %q (sorted, keyed cloud providers only)", got, want)
	}
}

// The zero value knows no key: catalogue expansions stay collapsed.
func TestProviderKeys_ZeroValue(t *testing.T) {
	var keys llm.ProviderKeys
	if keys.HasKey("groq") {
		t.Error("zero ProviderKeys: groq has no key")
	}
	if !keys.HasKey("ollama") {
		t.Error("zero ProviderKeys: ollama needs no key")
	}
}

// A misspelt provider name would silently hide the provider's models: it
// stops startup instead.
func TestNewProviderKeys_RejectsUnknownNames(t *testing.T) {
	for _, names := range [][]string{{"antropic"}, {"openai", "gpt"}, {"lmstudio"}} {
		_, err := llm.NewProviderKeys(names, noEnv)
		if err == nil {
			t.Fatalf("NewProviderKeys(%v): want an error", names)
		}
		if !strings.Contains(err.Error(), "keyed_providers") {
			t.Errorf("error %q should name the setting", err)
		}
	}
	for _, names := range [][]string{nil, {}, {"ollama", "anthropic"}} {
		if _, err := llm.NewProviderKeys(names, noEnv); err != nil {
			t.Errorf("NewProviderKeys(%v): %v", names, err)
		}
	}
}
