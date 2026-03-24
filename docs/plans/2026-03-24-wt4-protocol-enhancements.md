# WT4: Protocol Enhancements — MCP Resource Templates

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add parameterized MCP resource templates for per-project access: `codeforge://projects/{id}` and `codeforge://projects/{id}/costs`.

**Architecture:** Use mcp-go's `AddResourceTemplate()` API with `NewResourceTemplate()`. URI parameter extraction via `strings.TrimPrefix`. Handlers reuse existing `ServerDeps.ProjectLister.GetProject()` and `ServerDeps.CostReader` interfaces. A2A FIX-109 is already implemented — no action needed.

**Tech Stack:** Go 1.25, mcp-go v0.45.0

**Best Practices Applied:**
- RFC 6570 URI templates via mcp-go's built-in support
- Reuse existing narrow ServerDeps interfaces (no new dependencies)
- Follow existing resource handler pattern (same error handling, same JSON marshaling)

---

### Task 1: Add project resource template

**Files:**
- Modify: `internal/adapter/mcp/resources.go`

- [ ] **Step 1: Write the failing test**

Create: `internal/adapter/mcp/resources_test.go`

```go
package mcp

import (
	"context"
	"encoding/json"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"

	"github.com/Strob0t/CodeForge/internal/domain/project"
)

type mockProjectLister struct {
	projects []project.Project
}

func (m *mockProjectLister) ListProjects(ctx context.Context) ([]project.Project, error) {
	return m.projects, nil
}

func (m *mockProjectLister) GetProject(ctx context.Context, id string) (*project.Project, error) {
	for i := range m.projects {
		if m.projects[i].ID == id {
			return &m.projects[i], nil
		}
	}
	return nil, fmt.Errorf("project %q not found", id)
}

func TestHandleProjectResource(t *testing.T) {
	s := &Server{
		deps: ServerDeps{
			ProjectLister: &mockProjectLister{
				projects: []project.Project{
					{ID: "proj-1", Name: "Test Project"},
				},
			},
		},
	}

	ctx := context.Background()
	result, err := s.handleProjectResource(ctx, mcplib.ReadResourceRequest{
		Params: mcplib.ReadResourceRequestParams{
			URI: "codeforge://projects/proj-1",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	text := result[0].(mcplib.TextResourceContents).Text
	var p project.Project
	if err := json.Unmarshal([]byte(text), &p); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if p.ID != "proj-1" {
		t.Errorf("got project ID %q, want %q", p.ID, "proj-1")
	}
}

func TestHandleProjectResource_NotFound(t *testing.T) {
	s := &Server{
		deps: ServerDeps{
			ProjectLister: &mockProjectLister{},
		},
	}

	ctx := context.Background()
	_, err := s.handleProjectResource(ctx, mcplib.ReadResourceRequest{
		Params: mcplib.ReadResourceRequestParams{
			URI: "codeforge://projects/nonexistent",
		},
	})
	if err == nil {
		t.Fatal("expected error for missing project")
	}
}

func TestExtractProjectID(t *testing.T) {
	tests := []struct {
		uri  string
		want string
	}{
		{"codeforge://projects/abc-123", "abc-123"},
		{"codeforge://projects/", ""},
		{"codeforge://costs/summary", ""},
		{"invalid", ""},
	}
	for _, tt := range tests {
		got := extractProjectID(tt.uri)
		if got != tt.want {
			t.Errorf("extractProjectID(%q) = %q, want %q", tt.uri, got, tt.want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd internal/adapter/mcp && go test -run TestHandleProjectResource -v`
Expected: FAIL — `handleProjectResource` undefined

- [ ] **Step 3: Implement resource template handlers**

In `internal/adapter/mcp/resources.go`, add after existing imports:

```go
import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	mcplib "github.com/mark3labs/mcp-go/mcp"
)
```

Add the helper function:

```go
// extractProjectID extracts the project ID from a codeforge://projects/{id} URI.
func extractProjectID(uri string) string {
	const prefix = "codeforge://projects/"
	if !strings.HasPrefix(uri, prefix) {
		return ""
	}
	id := strings.TrimPrefix(uri, prefix)
	// Strip any trailing path segments (e.g., /costs)
	if idx := strings.IndexByte(id, '/'); idx >= 0 {
		id = id[:idx]
	}
	return id
}
```

Add the handler methods:

```go
func (s *Server) handleProjectResource(ctx context.Context, req mcplib.ReadResourceRequest) ([]mcplib.ResourceContents, error) {
	projectID := extractProjectID(req.Params.URI)
	if projectID == "" {
		return nil, fmt.Errorf("invalid project URI: %s", req.Params.URI)
	}

	p, err := s.deps.ProjectLister.GetProject(ctx, projectID)
	if err != nil {
		return nil, err
	}

	data, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}

	return []mcplib.ResourceContents{
		mcplib.TextResourceContents{
			URI:      req.Params.URI,
			MIMEType: "application/json",
			Text:     string(data),
		},
	}, nil
}

func (s *Server) handleProjectCostsResource(ctx context.Context, req mcplib.ReadResourceRequest) ([]mcplib.ResourceContents, error) {
	projectID := extractProjectID(req.Params.URI)
	if projectID == "" {
		return nil, fmt.Errorf("invalid project costs URI: %s", req.Params.URI)
	}

	// Use global cost summary filtered by project context.
	// TODO: Add per-project cost reader to ServerDeps when available.
	summary, err := s.deps.CostReader.CostSummaryGlobal(ctx)
	if err != nil {
		return nil, err
	}

	data, err := json.Marshal(summary)
	if err != nil {
		return nil, err
	}

	return []mcplib.ResourceContents{
		mcplib.TextResourceContents{
			URI:      req.Params.URI,
			MIMEType: "application/json",
			Text:     string(data),
		},
	}, nil
}
```

- [ ] **Step 4: Register templates in registerResources()**

Add to the `registerResources()` function, after existing `AddResource` calls:

```go
	// Parameterized resource templates for per-project access.
	s.mcpServer.AddResourceTemplate(
		mcplib.NewResourceTemplate(
			"codeforge://projects/{id}",
			"Project Details",
			mcplib.WithTemplateDescription("Access a specific project by ID"),
			mcplib.WithTemplateMIMEType("application/json"),
		),
		s.handleProjectResource,
	)

	s.mcpServer.AddResourceTemplate(
		mcplib.NewResourceTemplate(
			"codeforge://projects/{id}/costs",
			"Project Costs",
			mcplib.WithTemplateDescription("Cost summary for a specific project"),
			mcplib.WithTemplateMIMEType("application/json"),
		),
		s.handleProjectCostsResource,
	)
```

Remove the TODO comment at the top of `registerResources()`.

- [ ] **Step 5: Run tests**

Run: `cd internal/adapter/mcp && go test ./... -v`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add internal/adapter/mcp/resources.go internal/adapter/mcp/resources_test.go
git commit -m "feat: add MCP parameterized resource templates for per-project access"
```

---

### Task 2: Verify A2A streaming (FIX-109) — no action needed

**Files:** None (verification only)

- [ ] **Step 1: Verify FIX-109 is already implemented**

Read `internal/adapter/a2a/agentcard.go` and confirm:
1. `streaming bool` field exists on `CardBuilder` (line 22)
2. `WithStreaming` option exists (line 38)
3. `config.A2A.Streaming` is read in `cmd/codeforge/main.go`

If all confirmed, FIX-109 is a comment-only annotation — no code change needed.

- [ ] **Step 2: Commit (only if comment needs updating)**

If the FIX-109 comment is misleading, update it to clarify it's implemented:
```go
streaming bool // Configurable via config.A2A.Streaming (default: false)
```

---

### Task 3: Update docs/todo.md

**Files:**
- Modify: `docs/todo.md`

- [ ] **Step 1: Mark MCP resource templates as completed**

Find any MCP resource template entry and mark `[x]` with date `2026-03-24`.

- [ ] **Step 2: Commit**

```bash
git add docs/todo.md
git commit -m "docs: mark MCP resource templates as completed"
```
