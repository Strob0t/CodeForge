package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// KI-125 review: the providers whose API key LiteLLM holds, as names only
// (litellm.keyed_providers, CODEFORGE_LITELLM_KEYED_PROVIDERS). The
// production compose file derives them from the key variables with
// ${VAR:+name,}, so the list may end with a comma.

func TestKeyedProviders_DefaultEmpty(t *testing.T) {
	if names := Defaults().LiteLLM.KeyedProviders; len(names) != 0 {
		t.Fatalf("default keyed_providers = %v, want none", names)
	}
}

func TestKeyedProviders_Layering(t *testing.T) {
	path := filepath.Join(t.TempDir(), "codeforge.yaml")
	if err := os.WriteFile(path, []byte("litellm:\n  keyed_providers:\n    - anthropic\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := Defaults()
	if err := loadYAML(&cfg, path); err != nil {
		t.Fatalf("loadYAML: %v", err)
	}
	if want := []string{"anthropic"}; !slices.Equal(cfg.LiteLLM.KeyedProviders, want) {
		t.Fatalf("YAML keyed_providers = %v, want %v", cfg.LiteLLM.KeyedProviders, want)
	}

	t.Setenv("CODEFORGE_LITELLM_KEYED_PROVIDERS", "openai,groq,")
	loadEnv(&cfg)
	if want := []string{"openai", "groq"}; !slices.Equal(cfg.LiteLLM.KeyedProviders, want) {
		t.Fatalf("env keyed_providers = %v, want %v (env replaces YAML)", cfg.LiteLLM.KeyedProviders, want)
	}
}

func TestValidate_KeyedProviders(t *testing.T) {
	for name, tt := range map[string]struct {
		names   []string
		wantErr bool
	}{
		"none":          {nil, false},
		"known":         {[]string{"openai", "anthropic", "openrouter", "chutes"}, false},
		"misspelt":      {[]string{"antropic"}, true},
		"unknown among": {[]string{"openai", "gpt"}, true},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := Defaults()
			cfg.Auth.JWTSecret = strongTestSecret
			cfg.LiteLLM.KeyedProviders = tt.names
			err := validate(&cfg)
			if tt.wantErr != (err != nil) {
				t.Fatalf("validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), "litellm.keyed_providers") {
				t.Fatalf("error %q should name litellm.keyed_providers", err)
			}
		})
	}
}
