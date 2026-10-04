package llm

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// providerKeyVars maps the provider prefix of a model name ("groq" in
// "groq/llama-3.3-70b-versatile") to the variable holding its API key in
// LiteLLM's environment (litellm/config.yaml, the compose files). The
// worker's PROVIDER_KEY_MAP (workers/codeforge/provider_keys.py) is the same
// map; a worker test compares them.
var providerKeyVars = map[string]string{
	"openai":         "OPENAI_API_KEY",
	"anthropic":      "ANTHROPIC_API_KEY",
	"gemini":         "GEMINI_API_KEY",
	"groq":           "GROQ_API_KEY",
	"mistral":        "MISTRAL_API_KEY",
	"openrouter":     "OPENROUTER_API_KEY",
	"cerebras":       "CEREBRAS_API_KEY",
	"chutes":         "CHUTES_API_KEY",
	"aihubmix":       "AIHUBMIX_API_KEY",
	"deepseek":       "DEEPSEEK_API_KEY",
	"cohere":         "COHERE_API_KEY",
	"together_ai":    "TOGETHERAI_API_KEY",
	"fireworks_ai":   "FIREWORKS_API_KEY",
	"github_copilot": "GITHUB_TOKEN",
}

// keylessProviders are local model servers: they need no API key.
var keylessProviders = map[string]bool{"ollama": true, "lm_studio": true}

// ProviderKeys tells which providers' models the installation can call with
// its own API key (KI-125). LiteLLM holds the keys and reports nothing about
// them (/model/info strips them, and it expands some wildcard routes from its
// built-in catalogue with or without a key), so the Core learns the names
// from its configuration: the production compose file derives them from the
// key variables (${VAR:+name,}) without passing a key. The zero value knows
// no key.
type ProviderKeys struct {
	keyed map[string]bool
}

// NewProviderKeys returns the providers with a key: the providers named
// (litellm.keyed_providers) and those whose key variable is set in this
// process' environment (getenv; a development run with exported keys). A
// whitespace-only key is no key, and a name CodeForge does not know is an
// error: it would hide the provider's models without a word.
func NewProviderKeys(named []string, getenv func(string) string) (ProviderKeys, error) {
	keyed := make(map[string]bool, len(providerKeyVars))
	var unknown []string
	for _, name := range named {
		name = strings.ToLower(strings.TrimSpace(name))
		switch {
		case name == "":
		case keylessProviders[name]:
		case providerKeyVars[name] != "":
			keyed[name] = true
		default:
			unknown = append(unknown, name)
		}
	}
	if len(unknown) > 0 {
		return ProviderKeys{}, fmt.Errorf("litellm.keyed_providers: unknown provider(s) %s (known: %s)",
			strings.Join(unknown, ", "), strings.Join(knownKeyedProviders(), ", "))
	}
	for provider, envVar := range providerKeyVars {
		if strings.TrimSpace(getenv(envVar)) != "" {
			keyed[provider] = true
		}
	}
	return ProviderKeys{keyed: keyed}, nil
}

// knownKeyedProviders returns the providers CodeForge knows a key variable
// for, sorted.
func knownKeyedProviders() []string {
	return slices.Sorted(maps.Keys(providerKeyVars))
}

// Names returns the providers with a key, sorted.
func (k ProviderKeys) Names() []string {
	return slices.Sorted(maps.Keys(k.keyed))
}

// HasKey reports whether the models of provider (the prefix of a model name)
// can be called: a local server needs no key, and a provider CodeForge knows
// no key variable for is assumed to have one, as in the worker's routing
// (workers/codeforge/routing/key_filter.py).
func (k ProviderKeys) HasKey(provider string) bool {
	if keylessProviders[provider] {
		return true
	}
	if _, known := providerKeyVars[provider]; !known {
		return true
	}
	return k.keyed[provider]
}
