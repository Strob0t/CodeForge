package config

import (
	"strings"
	"testing"
	"time"
)

// KI-187: every git process of the Go Core has a deadline, local commands
// and the slower network commands (clone, fetch, pull, push) their own.
func TestGitTimeouts(t *testing.T) {
	cfg := Defaults()
	if cfg.Git.CommandTimeout != 2*time.Minute || cfg.Git.NetworkTimeout != 10*time.Minute {
		t.Fatalf("defaults: command %v, network %v; want 2m and 10m", cfg.Git.CommandTimeout, cfg.Git.NetworkTimeout)
	}

	t.Setenv("CODEFORGE_GIT_COMMAND_TIMEOUT", "30s")
	t.Setenv("CODEFORGE_GIT_NETWORK_TIMEOUT", "1h")
	cfg = Defaults()
	if err := loadEnv(&cfg); err != nil {
		t.Fatalf("loadEnv: %v", err)
	}
	if cfg.Git.CommandTimeout != 30*time.Second || cfg.Git.NetworkTimeout != time.Hour {
		t.Fatalf("from env: command %v, network %v", cfg.Git.CommandTimeout, cfg.Git.NetworkTimeout)
	}

	tests := []struct {
		name             string
		command, network time.Duration
		wantErr          string
	}{
		{"defaults", 2 * time.Minute, 10 * time.Minute, ""},
		{"one second each", time.Second, time.Second, ""},
		{"zero command deadline", 0, time.Minute, "git.command_timeout"},
		{"below one second", 999 * time.Millisecond, time.Minute, "git.command_timeout"},
		{"negative network deadline", time.Minute, -time.Second, "git.network_timeout"},
		{"zero network deadline", time.Minute, 0, "git.network_timeout"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Defaults()
			cfg.Auth.JWTSecret = strongTestSecret
			cfg.Git.CommandTimeout, cfg.Git.NetworkTimeout = tt.command, tt.network
			err := validate(&cfg)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validate() = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("validate() = %v, want an error naming %s", err, tt.wantErr)
			}
		})
	}
}
