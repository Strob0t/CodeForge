package config

import (
	"strings"
	"testing"
	"time"
)

// server.host (CODEFORGE_HOST) chooses the listen address; the default ""
// keeps all interfaces for the containers, live E2E binds to 127.0.0.1
// (KI-213).
func TestServer_ListenAddr(t *testing.T) {
	tests := []struct {
		host string
		want string
	}{
		{"", ":8080"},
		{"127.0.0.1", "127.0.0.1:8080"},
		{"0.0.0.0", "0.0.0.0:8080"},
		{"::1", "[::1]:8080"},
		{"localhost", "localhost:8080"},
	}
	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			s := Server{Host: tt.host, Port: "8080"}
			if got := s.ListenAddr(); got != tt.want {
				t.Errorf("ListenAddr() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLoadEnv_ServerHostAndSecureCookies(t *testing.T) {
	t.Setenv("CODEFORGE_HOST", "127.0.0.1")
	t.Setenv("CODEFORGE_FORCE_SECURE_COOKIES", "true")
	cfg := Defaults()
	mustLoadEnv(t, &cfg)
	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("server.host = %q, want 127.0.0.1", cfg.Server.Host)
	}
	if !cfg.Server.ForceSecureCookies {
		t.Error("server.force_secure_cookies not set from CODEFORGE_FORCE_SECURE_COOKIES")
	}
	if d := Defaults(); d.Server.Host != "" || d.Server.ForceSecureCookies {
		t.Errorf("defaults: host=%q force_secure_cookies=%v, want all interfaces and false", d.Server.Host, d.Server.ForceSecureCookies)
	}
}

// git.operation_timeout bounds the synchronous clone, setup and pull API
// calls instead of the 30 s request timeout (KI-213).
func TestGitOperationTimeout(t *testing.T) {
	if got := Defaults().Git.OperationTimeout; got != 30*time.Minute {
		t.Fatalf("default git.operation_timeout = %s, want 30m", got)
	}
	t.Setenv("CODEFORGE_GIT_OPERATION_TIMEOUT", "2h")
	cfg := Defaults()
	mustLoadEnv(t, &cfg)
	if cfg.Git.OperationTimeout != 2*time.Hour {
		t.Fatalf("git.operation_timeout = %s, want 2h", cfg.Git.OperationTimeout)
	}
	for _, d := range []time.Duration{0, -time.Second} {
		cfg := Defaults()
		cfg.Auth.Enabled = false
		cfg.Git.OperationTimeout = d
		if err := validate(&cfg); err == nil || !strings.Contains(err.Error(), "git.operation_timeout") {
			t.Errorf("validate(%s) = %v, want a git.operation_timeout error", d, err)
		}
	}
}

func TestValidate_ServerHost(t *testing.T) {
	tests := []struct {
		host    string
		wantErr bool
	}{
		{"", false},
		{"127.0.0.1", false},
		{"::", false},
		{"localhost", false},
		{"127.0.0.1:8080", true},
		{"codeforge.example.com", true},
		{" 127.0.0.1", true},
	}
	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			cfg := Defaults()
			cfg.Auth.Enabled = false // the auth checks need secrets this test does not set
			cfg.Server.Host = tt.host
			err := validate(&cfg)
			if tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), "server.host") {
					t.Fatalf("validate = %v, want a server.host error", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("validate: %v", err)
			}
		})
	}
}
