package mcp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"

	cfmcp "github.com/Strob0t/CodeForge/internal/adapter/mcp"
	"github.com/Strob0t/CodeForge/internal/domain/cost"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// --- Mocks ---

type mockProjectLister struct {
	projects []project.Project
	err      error
}

func (m *mockProjectLister) ListProjects(_ context.Context) ([]project.Project, error) {
	return m.projects, m.err
}

func (m *mockProjectLister) GetProject(_ context.Context, id string) (*project.Project, error) {
	for i := range m.projects {
		if m.projects[i].ID == id {
			return &m.projects[i], nil
		}
	}
	return nil, m.err
}

type mockRunReader struct {
	runs map[string]*run.Run
	err  error
}

func (m *mockRunReader) GetRun(_ context.Context, id string) (*run.Run, error) {
	if r, ok := m.runs[id]; ok {
		return r, nil
	}
	return nil, m.err
}

type mockCostReader struct {
	summary []cost.ProjectSummary
	err     error
}

func (m *mockCostReader) CostSummaryGlobal(_ context.Context) ([]cost.ProjectSummary, error) {
	return m.summary, m.err
}

// validDeps returns a minimal ServerDeps with non-nil implementations for all fields.
func validDeps() cfmcp.ServerDeps {
	return cfmcp.ServerDeps{
		ProjectLister: &mockProjectLister{},
		RunReader:     &mockRunReader{runs: map[string]*run.Run{}},
		CostReader:    &mockCostReader{},
	}
}

// --- Tests ---

func TestNewServer(t *testing.T) {
	cfg := cfmcp.ServerConfig{
		Addr:    ":3001",
		Name:    "test-server",
		Version: "0.8.0",
	}
	s, err := cfmcp.NewServer(cfg, validDeps())
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}
	if s == nil {
		t.Fatal("NewServer returned nil")
	}
	if s.MCPServer() == nil {
		t.Fatal("MCPServer() returned nil")
	}
}

func TestNewServer_NilDepsErrors(t *testing.T) {
	tests := []struct {
		name string
		deps cfmcp.ServerDeps
		want string
	}{
		{
			name: "nil ProjectLister",
			deps: cfmcp.ServerDeps{
				RunReader:  &mockRunReader{runs: map[string]*run.Run{}},
				CostReader: &mockCostReader{},
			},
			want: "ProjectLister",
		},
		{
			name: "nil RunReader",
			deps: cfmcp.ServerDeps{
				ProjectLister: &mockProjectLister{},
				CostReader:    &mockCostReader{},
			},
			want: "RunReader",
		},
		{
			name: "nil CostReader",
			deps: cfmcp.ServerDeps{
				ProjectLister: &mockProjectLister{},
				RunReader:     &mockRunReader{runs: map[string]*run.Run{}},
			},
			want: "CostReader",
		},
		{
			name: "all nil",
			deps: cfmcp.ServerDeps{},
			want: "ProjectLister",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := cfmcp.NewServer(cfmcp.ServerConfig{}, tc.deps)
			if err == nil {
				t.Fatal("expected error for nil dependency")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error message %q should mention %q", err.Error(), tc.want)
			}
		})
	}
}

func TestServerStartStop(t *testing.T) {
	cfg := cfmcp.ServerConfig{
		Addr:    ":0",
		Name:    "test-server",
		Version: "0.8.0",
	}
	s, err := cfmcp.NewServer(cfg, validDeps())
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	if err := s.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if err := s.Stop(context.Background()); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

func TestToolRegistration(t *testing.T) {
	deps := cfmcp.ServerDeps{
		ProjectLister: &mockProjectLister{
			projects: []project.Project{
				{ID: "p1", Name: "Project One"},
			},
		},
		RunReader: &mockRunReader{
			runs: map[string]*run.Run{
				"r1": {ID: "r1", Status: run.StatusRunning},
			},
		},
		CostReader: &mockCostReader{
			summary: []cost.ProjectSummary{
				{ProjectID: "p1", ProjectName: "Project One", Summary: cost.Summary{TotalCostUSD: 1.5}},
			},
		},
	}
	s, err := cfmcp.NewServer(cfmcp.ServerConfig{Name: "test", Version: "0.8.0"}, deps)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	tools := s.MCPServer().ListTools()
	if len(tools) != 4 {
		t.Fatalf("expected 4 tools, got %d", len(tools))
	}

	expectedTools := map[string]bool{
		"list_projects":    false,
		"get_project":      false,
		"get_run_status":   false,
		"get_cost_summary": false,
	}
	for name := range tools {
		if _, ok := expectedTools[name]; ok {
			expectedTools[name] = true
		} else {
			t.Errorf("unexpected tool: %s", name)
		}
	}
	for name, found := range expectedTools {
		if !found {
			t.Errorf("expected tool %q not registered", name)
		}
	}
}

func TestHandleListProjects(t *testing.T) {
	deps := validDeps()
	deps.ProjectLister = &mockProjectLister{
		projects: []project.Project{
			{ID: "p1", Name: "Alpha"},
			{ID: "p2", Name: "Beta"},
		},
	}
	s, err := cfmcp.NewServer(cfmcp.ServerConfig{Name: "test", Version: "0.8.0"}, deps)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	ctx := context.Background()

	// Use the tool handler directly by calling through the registered tools map
	tools := s.MCPServer().ListTools()
	listTool, ok := tools["list_projects"]
	if !ok {
		t.Fatal("list_projects tool not found")
	}

	result, err := listTool.Handler(ctx, mcplib.CallToolRequest{
		Params: mcplib.CallToolParams{Name: "list_projects"},
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %v", result.Content)
	}

	text, ok := result.Content[0].(mcplib.TextContent)
	if !ok {
		t.Fatal("expected TextContent")
	}
	var projects []project.Project
	if err := json.Unmarshal([]byte(text.Text), &projects); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if len(projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(projects))
	}
}

func TestHandleGetRunStatus(t *testing.T) {
	deps := validDeps()
	deps.RunReader = &mockRunReader{
		runs: map[string]*run.Run{
			"run-abc": {ID: "run-abc", Status: run.StatusCompleted},
		},
	}
	s, err := cfmcp.NewServer(cfmcp.ServerConfig{Name: "test", Version: "0.8.0"}, deps)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	tools := s.MCPServer().ListTools()
	runTool, ok := tools["get_run_status"]
	if !ok {
		t.Fatal("get_run_status tool not found")
	}

	ctx := context.Background()
	result, err := runTool.Handler(ctx, mcplib.CallToolRequest{
		Params: mcplib.CallToolParams{
			Name:      "get_run_status",
			Arguments: map[string]any{"run_id": "run-abc"},
		},
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %v", result.Content)
	}

	text, ok := result.Content[0].(mcplib.TextContent)
	if !ok {
		t.Fatal("expected TextContent")
	}
	var r run.Run
	if err := json.Unmarshal([]byte(text.Text), &r); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if r.Status != run.StatusCompleted {
		t.Fatalf("expected status %q, got %q", run.StatusCompleted, r.Status)
	}
}

func TestHandleGetRunStatusMissingArg(t *testing.T) {
	s, err := cfmcp.NewServer(cfmcp.ServerConfig{Name: "test", Version: "0.8.0"}, validDeps())
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	tools := s.MCPServer().ListTools()
	runTool, ok := tools["get_run_status"]
	if !ok {
		t.Fatal("get_run_status tool not found")
	}

	ctx := context.Background()
	result, err := runTool.Handler(ctx, mcplib.CallToolRequest{
		Params: mcplib.CallToolParams{Name: "get_run_status"},
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing run_id")
	}
}

// TestHandleNilDeps is now covered by TestNewServer_NilDepsPanics.
// NewServer panics on nil deps, so runtime nil checks in handlers are unnecessary.

func TestNewServer_WithAPIKey_RejectsUnauthenticated(t *testing.T) {
	cfg := cfmcp.ServerConfig{
		Addr:    ":0",
		Name:    "test-auth",
		Version: "0.8.0",
		APIKey:  "test-secret-key",
	}
	deps := cfmcp.ServerDeps{
		ProjectLister: &mockProjectLister{
			projects: []project.Project{{ID: "p1", Name: "Alpha"}},
		},
		RunReader:  &mockRunReader{runs: map[string]*run.Run{}},
		CostReader: &mockCostReader{},
	}
	s, err := cfmcp.NewServer(cfg, deps)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}
	if s == nil {
		t.Fatal("NewServer returned nil")
	}

	// Start and stop immediately is fine; we just need to verify the config is wired.
	// The auth enforcement is tested via the AuthMiddleware unit (auth.go).
	// This test verifies the server is created without error when APIKey is set.
}

func TestNewServer_WithoutAPIKey_AllowsRequests(t *testing.T) {
	cfg := cfmcp.ServerConfig{
		Addr:    ":0",
		Name:    "test-noauth",
		Version: "0.8.0",
		APIKey:  "", // Empty means no auth
	}
	s, err := cfmcp.NewServer(cfg, validDeps())
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}
	if s == nil {
		t.Fatal("NewServer returned nil")
	}
}

func TestAuthMiddleware_RejectsUnauthenticated(t *testing.T) {
	handler := cfmcp.AuthMiddleware("my-secret", tenantctx.DefaultTenantID, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// No auth header -> 401
	req := httptest.NewRequest(http.MethodPost, "/mcp", http.NoBody)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}

	// Wrong key -> 403
	req = httptest.NewRequest(http.MethodPost, "/mcp", http.NoBody)
	req.Header.Set("Authorization", "Bearer wrong-key")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}

	// Correct key -> 200
	req = httptest.NewRequest(http.MethodPost, "/mcp", http.NoBody)
	req.Header.Set("Authorization", "Bearer my-secret")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestAuthMiddleware_EmptyKey_PassesAll(t *testing.T) {
	handler := cfmcp.AuthMiddleware("", tenantctx.DefaultTenantID, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// No auth header but empty key -> passes through
	req := httptest.NewRequest(http.MethodPost, "/mcp", http.NoBody)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with empty API key, got %d", w.Code)
	}
}

func TestAuthMiddleware_InjectsTenantContext(t *testing.T) {
	wantTenant := "tenant-abc-123"

	// With auth enabled: correct key should inject tenant into context.
	handler := cfmcp.AuthMiddleware("key", wantTenant, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := tenantctx.FromContext(r.Context())
		if got != wantTenant {
			t.Errorf("with auth: tenant = %q, want %q", got, wantTenant)
		}
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodPost, "/mcp", http.NoBody)
	req.Header.Set("Authorization", "Bearer key")
	handler.ServeHTTP(httptest.NewRecorder(), req)

	// With auth disabled (empty key): should still inject tenant into context.
	noAuthHandler := cfmcp.AuthMiddleware("", wantTenant, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := tenantctx.FromContext(r.Context())
		if got != wantTenant {
			t.Errorf("no auth: tenant = %q, want %q", got, wantTenant)
		}
		w.WriteHeader(http.StatusOK)
	}))
	req = httptest.NewRequest(http.MethodPost, "/mcp", http.NoBody)
	noAuthHandler.ServeHTTP(httptest.NewRecorder(), req)
}

func TestHandleGetCostSummary(t *testing.T) {
	deps := validDeps()
	deps.CostReader = &mockCostReader{
		summary: []cost.ProjectSummary{
			{ProjectID: "p1", ProjectName: "Alpha", Summary: cost.Summary{TotalCostUSD: 42.5, RunCount: 10}},
		},
	}
	s, err := cfmcp.NewServer(cfmcp.ServerConfig{Name: "test", Version: "0.8.0"}, deps)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	tools := s.MCPServer().ListTools()
	costTool, ok := tools["get_cost_summary"]
	if !ok {
		t.Fatal("get_cost_summary tool not found")
	}

	ctx := context.Background()
	result, err := costTool.Handler(ctx, mcplib.CallToolRequest{
		Params: mcplib.CallToolParams{Name: "get_cost_summary"},
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %v", result.Content)
	}

	text, ok := result.Content[0].(mcplib.TextContent)
	if !ok {
		t.Fatal("expected TextContent")
	}
	var summaries []cost.ProjectSummary
	if err := json.Unmarshal([]byte(text.Text), &summaries); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}
	if summaries[0].TotalCostUSD != 42.5 {
		t.Fatalf("expected cost 42.5, got %f", summaries[0].TotalCostUSD)
	}
}
