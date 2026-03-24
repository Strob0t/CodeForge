package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"

	"github.com/Strob0t/CodeForge/internal/domain/cost"
	"github.com/Strob0t/CodeForge/internal/domain/project"
)

type stubProjectLister struct {
	projects []project.Project
}

func (m *stubProjectLister) ListProjects(_ context.Context) ([]project.Project, error) {
	return m.projects, nil
}

func (m *stubProjectLister) GetProject(_ context.Context, id string) (*project.Project, error) {
	for i := range m.projects {
		if m.projects[i].ID == id {
			return &m.projects[i], nil
		}
	}
	return nil, fmt.Errorf("project %q not found", id)
}

type stubCostReader struct {
	summary []cost.ProjectSummary
}

func (m *stubCostReader) CostSummaryGlobal(_ context.Context) ([]cost.ProjectSummary, error) {
	return m.summary, nil
}

func TestHandleProjectResource(t *testing.T) {
	s := &Server{
		deps: ServerDeps{
			ProjectLister: &stubProjectLister{
				projects: []project.Project{
					{ID: "proj-1", Name: "Test Project"},
				},
			},
		},
	}

	ctx := context.Background()
	result, err := s.handleProjectResource(ctx, mcplib.ReadResourceRequest{
		Params: mcplib.ReadResourceParams{
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
			ProjectLister: &stubProjectLister{},
		},
	}

	ctx := context.Background()
	_, err := s.handleProjectResource(ctx, mcplib.ReadResourceRequest{
		Params: mcplib.ReadResourceParams{
			URI: "codeforge://projects/nonexistent",
		},
	})
	if err == nil {
		t.Fatal("expected error for missing project")
	}
}

func TestHandleProjectCostsResource(t *testing.T) {
	s := &Server{
		deps: ServerDeps{
			ProjectLister: &stubProjectLister{
				projects: []project.Project{
					{ID: "proj-1", Name: "Test Project"},
				},
			},
			CostReader: &stubCostReader{
				summary: []cost.ProjectSummary{
					{ProjectID: "proj-1", ProjectName: "Test Project", Summary: cost.Summary{TotalCostUSD: 12.5, RunCount: 3}},
				},
			},
		},
	}

	ctx := context.Background()
	result, err := s.handleProjectCostsResource(ctx, mcplib.ReadResourceRequest{
		Params: mcplib.ReadResourceParams{
			URI: "codeforge://projects/proj-1/costs",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	text := result[0].(mcplib.TextResourceContents).Text
	var summaries []cost.ProjectSummary
	if err := json.Unmarshal([]byte(text), &summaries); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}
	if summaries[0].TotalCostUSD != 12.5 {
		t.Errorf("got cost %f, want 12.5", summaries[0].TotalCostUSD)
	}
}

func TestHandleProjectCostsResource_InvalidURI(t *testing.T) {
	s := &Server{
		deps: ServerDeps{
			CostReader: &stubCostReader{},
		},
	}

	ctx := context.Background()
	_, err := s.handleProjectCostsResource(ctx, mcplib.ReadResourceRequest{
		Params: mcplib.ReadResourceParams{
			URI: "codeforge://invalid",
		},
	})
	if err == nil {
		t.Fatal("expected error for invalid URI")
	}
}

func TestExtractProjectID(t *testing.T) {
	tests := []struct {
		uri  string
		want string
	}{
		{"codeforge://projects/abc-123", "abc-123"},
		{"codeforge://projects/abc-123/costs", "abc-123"},
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
