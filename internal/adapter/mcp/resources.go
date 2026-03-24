package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	mcplib "github.com/mark3labs/mcp-go/mcp"
)

// registerResources registers all MCP resources and resource templates on the server.
func (s *Server) registerResources() {
	s.mcpServer.AddResource(
		mcplib.NewResource(
			"codeforge://projects",
			"Project List",
			mcplib.WithResourceDescription("List of all CodeForge projects"),
			mcplib.WithMIMEType("application/json"),
		),
		s.handleProjectsResource,
	)

	s.mcpServer.AddResource(
		mcplib.NewResource(
			"codeforge://costs/summary",
			"Cost Summary",
			mcplib.WithResourceDescription("Global cost summary across all projects"),
			mcplib.WithMIMEType("application/json"),
		),
		s.handleCostSummaryResource,
	)

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
}

func (s *Server) handleProjectsResource(ctx context.Context, req mcplib.ReadResourceRequest) ([]mcplib.ResourceContents, error) {
	if s.deps.ProjectLister == nil {
		return []mcplib.ResourceContents{
			mcplib.TextResourceContents{
				URI:      req.Params.URI,
				MIMEType: "application/json",
				Text:     `{"error":"project lister not configured"}`,
			},
		}, nil
	}
	projects, err := s.deps.ProjectLister.ListProjects(ctx)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(projects)
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

func (s *Server) handleCostSummaryResource(ctx context.Context, req mcplib.ReadResourceRequest) ([]mcplib.ResourceContents, error) {
	if s.deps.CostReader == nil {
		return []mcplib.ResourceContents{
			mcplib.TextResourceContents{
				URI:      req.Params.URI,
				MIMEType: "application/json",
				Text:     `{"error":"cost reader not configured"}`,
			},
		}, nil
	}
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

// extractProjectID extracts the project ID from a codeforge://projects/{id} URI.
func extractProjectID(uri string) string {
	const prefix = "codeforge://projects/"
	if !strings.HasPrefix(uri, prefix) {
		return ""
	}
	id := strings.TrimPrefix(uri, prefix)
	// Strip any trailing path segments (e.g., /costs).
	if idx := strings.IndexByte(id, '/'); idx >= 0 {
		id = id[:idx]
	}
	return id
}

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
