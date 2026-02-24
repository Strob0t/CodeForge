package mcp

import (
	"context"
	"encoding/json"

	mcplib "github.com/mark3labs/mcp-go/mcp"
)

// registerResources registers all MCP resources on the server.
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
