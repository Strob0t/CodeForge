package service

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/mcp"
)

func TestNewMCPService(t *testing.T) {
	svc := NewMCPService(&config.MCP{}, &config.Limits{MCPTestTimeout: 10 * time.Second})
	if svc == nil {
		t.Fatal("expected non-nil MCPService")
	}
	if got := len(svc.List()); got != 0 {
		t.Fatalf("expected 0 servers, got %d", got)
	}
}

func TestMCP_Register(t *testing.T) {
	svc := NewMCPService(&config.MCP{}, &config.Limits{MCPTestTimeout: 10 * time.Second})

	def := mcp.ServerDef{
		Name:      "test-server",
		Transport: mcp.TransportStdio,
		Command:   "echo",
		Enabled:   true,
	}

	if err := svc.Register(def); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	servers := svc.List()
	if len(servers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(servers))
	}
	if servers[0].Name != "test-server" {
		t.Errorf("expected name %q, got %q", "test-server", servers[0].Name)
	}
	if servers[0].ID == "" {
		t.Error("expected generated ID, got empty")
	}
	if servers[0].Status != mcp.ServerStatusRegistered {
		t.Errorf("expected status %q, got %q", mcp.ServerStatusRegistered, servers[0].Status)
	}
}

func TestRegisterDuplicate(t *testing.T) {
	svc := NewMCPService(&config.MCP{}, &config.Limits{MCPTestTimeout: 10 * time.Second})

	def := mcp.ServerDef{
		ID:        "dup-id",
		Name:      "test-server",
		Transport: mcp.TransportStdio,
		Command:   "echo",
		Enabled:   true,
	}

	if err := svc.Register(def); err != nil {
		t.Fatalf("first register: %v", err)
	}

	err := svc.Register(def)
	if err == nil {
		t.Fatal("expected error on duplicate registration")
	}
	if !errors.Is(err, domain.ErrConflict) {
		t.Errorf("expected ErrConflict, got: %v", err)
	}
}

func TestRegisterValidation(t *testing.T) {
	svc := NewMCPService(&config.MCP{}, &config.Limits{MCPTestTimeout: 10 * time.Second})

	tests := []struct {
		name string
		def  mcp.ServerDef
	}{
		{
			name: "missing name",
			def:  mcp.ServerDef{Transport: mcp.TransportStdio, Command: "echo"},
		},
		{
			name: "missing transport",
			def:  mcp.ServerDef{Name: "test"},
		},
		{
			name: "invalid transport",
			def:  mcp.ServerDef{Name: "test", Transport: "grpc"},
		},
		{
			name: "stdio without command",
			def:  mcp.ServerDef{Name: "test", Transport: mcp.TransportStdio},
		},
		{
			name: "sse without url",
			def:  mcp.ServerDef{Name: "test", Transport: mcp.TransportSSE},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := svc.Register(tc.def)
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !errors.Is(err, domain.ErrValidation) {
				t.Errorf("expected ErrValidation, got: %v", err)
			}
		})
	}
}

func TestListAndGet(t *testing.T) {
	svc := NewMCPService(&config.MCP{}, &config.Limits{MCPTestTimeout: 10 * time.Second})

	defs := []mcp.ServerDef{
		{ID: "aaa", Name: "server-a", Transport: mcp.TransportStdio, Command: "a", Enabled: true},
		{ID: "bbb", Name: "server-b", Transport: mcp.TransportSSE, URL: "http://b", Enabled: false},
	}
	for _, d := range defs {
		if err := svc.Register(d); err != nil {
			t.Fatalf("register %s: %v", d.ID, err)
		}
	}

	// List returns all servers sorted by ID.
	servers := svc.List()
	if len(servers) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(servers))
	}
	if servers[0].ID != "aaa" || servers[1].ID != "bbb" {
		t.Errorf("expected sorted order [aaa, bbb], got [%s, %s]", servers[0].ID, servers[1].ID)
	}

	// Get existing.
	got, err := svc.Get("aaa")
	if err != nil {
		t.Fatalf("get aaa: %v", err)
	}
	if got.Name != "server-a" {
		t.Errorf("expected name %q, got %q", "server-a", got.Name)
	}

	// Get non-existent.
	_, err = svc.Get("zzz")
	if err == nil {
		t.Fatal("expected error for non-existent server")
	}
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestRemove(t *testing.T) {
	svc := NewMCPService(&config.MCP{}, &config.Limits{MCPTestTimeout: 10 * time.Second})

	def := mcp.ServerDef{
		ID:        "rm-me",
		Name:      "removable",
		Transport: mcp.TransportStdio,
		Command:   "echo",
		Enabled:   true,
	}
	if err := svc.Register(def); err != nil {
		t.Fatalf("register: %v", err)
	}

	// Remove existing.
	if err := svc.Remove("rm-me"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if len(svc.List()) != 0 {
		t.Error("expected 0 servers after removal")
	}

	// Remove non-existent.
	err := svc.Remove("rm-me")
	if err == nil {
		t.Fatal("expected error removing non-existent server")
	}
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestResolveForRun(t *testing.T) {
	svc := NewMCPService(&config.MCP{}, &config.Limits{MCPTestTimeout: 10 * time.Second})

	defs := []mcp.ServerDef{
		{ID: "enabled-1", Name: "e1", Transport: mcp.TransportStdio, Command: "e1", Enabled: true},
		{ID: "disabled-1", Name: "d1", Transport: mcp.TransportStdio, Command: "d1", Enabled: false},
		{ID: "enabled-2", Name: "e2", Transport: mcp.TransportSSE, URL: "http://e2", Enabled: true},
	}
	for _, d := range defs {
		if err := svc.Register(d); err != nil {
			t.Fatalf("register %s: %v", d.ID, err)
		}
	}

	resolved := svc.ResolveForRun(context.Background(), "proj-1", "coder")
	if len(resolved) != 2 {
		t.Fatalf("expected 2 enabled servers, got %d", len(resolved))
	}
	for _, r := range resolved {
		if !r.Enabled {
			t.Errorf("resolved server %q should be enabled", r.ID)
		}
	}
}

func TestMCP_LoadFromDirectory(t *testing.T) {
	dir := t.TempDir()

	// Write valid YAML server defs.
	stdio := `name: file-server
transport: stdio
command: /usr/bin/test
enabled: true
`
	sse := `id: sse-from-file
name: sse-server
transport: sse
url: http://localhost:9090
enabled: true
env:
  TOKEN: secret
`
	if err := os.WriteFile(filepath.Join(dir, "stdio.yaml"), []byte(stdio), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sse.yml"), []byte(sse), 0o644); err != nil {
		t.Fatal(err)
	}
	// Non-YAML file should be ignored.
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("ignore me"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := NewMCPService(&config.MCP{ServersDir: dir}, &config.Limits{MCPTestTimeout: 10 * time.Second})

	servers := svc.List()
	if len(servers) != 2 {
		t.Fatalf("expected 2 servers from directory, got %d", len(servers))
	}

	// Check the SSE server loaded with its explicit ID.
	got, err := svc.Get("sse-from-file")
	if err != nil {
		t.Fatalf("get sse-from-file: %v", err)
	}
	if got.URL != "http://localhost:9090" {
		t.Errorf("expected URL %q, got %q", "http://localhost:9090", got.URL)
	}
	if got.Env["TOKEN"] != "secret" {
		t.Errorf("expected env TOKEN=secret, got %q", got.Env["TOKEN"])
	}
}

func TestMCP_LoadFromDirectoryMissing(t *testing.T) {
	svc := NewMCPService(&config.MCP{}, &config.Limits{MCPTestTimeout: 10 * time.Second})

	// Loading from a non-existent directory returns nil (not an error).
	err := svc.LoadFromDirectory("/nonexistent/path/mcp-servers")
	if err != nil {
		t.Fatalf("expected nil error for missing directory, got: %v", err)
	}
}

// TestLoadFromDirectorySkipsInvalidFiles (KI-100 review): an invalid file
// used to stop the loading, so the definitions after it were missing. Each
// invalid file is now logged with its name and reason and skipped.
func TestLoadFromDirectorySkipsInvalidFiles(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"a-bad-syntax.yaml":  "name: [unclosed\n",
		"b-invalid.yaml":     "name: \"\"\ntransport: stdio\n",
		"c-good.yaml":        "id: good\nname: good\ntransport: stdio\ncommand: mcp-good\nenabled: true\n",
		"d-duplicate.yaml":   "id: good\nname: again\ntransport: stdio\ncommand: mcp-again\n",
		"e-bad-url.yml":      "name: remote\ntransport: sse\nurl: ftp://mcp.example/\n",
		"f-also-good.yml":    "id: also-good\nname: also\ntransport: sse\nurl: http://127.0.0.1:6280/sse\n",
		"g-not-a-server.txt": "ignored",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	svc := NewMCPService(&config.MCP{}, &config.Limits{MCPTestTimeout: 10 * time.Second})
	if err := svc.LoadFromDirectory(dir); err != nil {
		t.Fatalf("LoadFromDirectory = %v, want the invalid files skipped", err)
	}

	if got := svc.List(); len(got) != 2 || got[0].ID != "also-good" || got[1].ID != "good" || got[1].Command != "mcp-good" {
		t.Fatalf("servers = %+v, want good and also-good", got)
	}
	for _, name := range []string{"a-bad-syntax.yaml", "b-invalid.yaml", "d-duplicate.yaml", "e-bad-url.yml"} {
		if !strings.Contains(logs.String(), name) {
			t.Errorf("the log does not name the skipped file %s:\n%s", name, logs.String())
		}
	}
	if !strings.Contains(logs.String(), "level=ERROR") || !strings.Contains(logs.String(), "url must be an http or https URL") {
		t.Errorf("the log does not give the reason at error level:\n%s", logs.String())
	}
}
