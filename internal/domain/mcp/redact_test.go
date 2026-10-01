package mcp

import (
	"errors"
	"maps"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain"
)

// KI-71 review: env variables and headers of an MCP server carry
// credentials. The API shows which are set (keys, RedactedValue), never a
// value; a client that sends RedactedValue back keeps the stored value.

func TestServerDef_Redacted(t *testing.T) {
	stored := ServerDef{
		ID: "s1", Name: "github", Transport: TransportStdio, Command: "mcp-github", Args: []string{"--stdio"},
		Env:     map[string]string{"GITHUB_TOKEN": "ghp_secret", "EMPTY": ""},
		Headers: map[string]string{"Authorization": "Bearer secret"},
	}

	got := stored.Redacted()

	if want := map[string]string{"GITHUB_TOKEN": RedactedValue, "EMPTY": ""}; !maps.Equal(got.Env, want) {
		t.Errorf("Env = %v, want %v", got.Env, want)
	}
	if want := map[string]string{"Authorization": RedactedValue}; !maps.Equal(got.Headers, want) {
		t.Errorf("Headers = %v, want %v", got.Headers, want)
	}
	if got.ID != "s1" || got.Command != "mcp-github" || len(got.Args) != 1 {
		t.Errorf("Redacted changed other fields: %+v", got)
	}
	if stored.Env["GITHUB_TOKEN"] != "ghp_secret" || stored.Headers["Authorization"] != "Bearer secret" {
		t.Errorf("Redacted changed the stored definition: %+v", stored)
	}
	if empty := (&ServerDef{}).Redacted(); empty.Env != nil || empty.Headers != nil {
		t.Errorf("Redacted of a server without env and headers = %+v, want nil maps", empty)
	}
}

func TestServerDef_KeepRedacted(t *testing.T) {
	stored := &ServerDef{
		Env:     map[string]string{"TOKEN": "secret", "REGION": "eu"},
		Headers: map[string]string{"Authorization": "Bearer secret"},
	}
	tests := []struct {
		name        string
		env         map[string]string
		headers     map[string]string
		stored      *ServerDef
		wantEnv     map[string]string
		wantHeaders map[string]string
		wantErr     bool
	}{
		{
			name:        "sent back unchanged",
			env:         map[string]string{"TOKEN": RedactedValue, "REGION": RedactedValue},
			headers:     map[string]string{"Authorization": RedactedValue},
			stored:      stored,
			wantEnv:     map[string]string{"TOKEN": "secret", "REGION": "eu"},
			wantHeaders: map[string]string{"Authorization": "Bearer secret"},
		},
		{
			name:    "one value changed, one removed, one added",
			env:     map[string]string{"TOKEN": "new-secret", "NEW": "value"},
			headers: map[string]string{"Authorization": RedactedValue},
			stored:  stored,
			wantEnv: map[string]string{"TOKEN": "new-secret", "NEW": "value"}, wantHeaders: map[string]string{"Authorization": "Bearer secret"},
		},
		{name: "nothing redacted", env: map[string]string{"A": "b"}, stored: nil, wantEnv: map[string]string{"A": "b"}},
		{name: "redacted value of a new key", env: map[string]string{"OTHER": RedactedValue}, stored: stored, wantErr: true},
		{name: "redacted value without a stored server", headers: map[string]string{"Authorization": RedactedValue}, stored: nil, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def := &ServerDef{Env: tt.env, Headers: tt.headers}
			err := def.KeepRedacted(tt.stored)
			if tt.wantErr {
				if !errors.Is(err, domain.ErrValidation) {
					t.Fatalf("KeepRedacted = %v, want domain.ErrValidation", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("KeepRedacted: %v", err)
			}
			if !maps.Equal(def.Env, tt.wantEnv) || !maps.Equal(def.Headers, tt.wantHeaders) {
				t.Errorf("env %v, headers %v; want %v, %v", def.Env, def.Headers, tt.wantEnv, tt.wantHeaders)
			}
		})
	}
}
