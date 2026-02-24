package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// registerTools registers all MCP tools on the server.
func (s *Server) registerTools() {
	s.mcpServer.AddTools(
		s.listProjectsTool(),
		s.getProjectTool(),
		s.getRunStatusTool(),
		s.getCostSummaryTool(),
	)
}

func (s *Server) listProjectsTool() mcpserver.ServerTool {
	tool := mcplib.NewTool("list_projects",
		mcplib.WithDescription("List all projects managed by CodeForge"),
	)
	return mcpserver.ServerTool{
		Tool:    tool,
		Handler: s.handleListProjects,
	}
}

func (s *Server) getProjectTool() mcpserver.ServerTool {
	tool := mcplib.NewTool("get_project",
		mcplib.WithDescription("Get details of a specific project by ID"),
		mcplib.WithString("project_id",
			mcplib.Required(),
			mcplib.Description("The project ID to look up"),
		),
	)
	return mcpserver.ServerTool{
		Tool:    tool,
		Handler: s.handleGetProject,
	}
}

func (s *Server) getRunStatusTool() mcpserver.ServerTool {
	tool := mcplib.NewTool("get_run_status",
		mcplib.WithDescription("Get the status of an agent run by run ID"),
		mcplib.WithString("run_id",
			mcplib.Required(),
			mcplib.Description("The run ID to check"),
		),
	)
	return mcpserver.ServerTool{
		Tool:    tool,
		Handler: s.handleGetRunStatus,
	}
}

func (s *Server) getCostSummaryTool() mcpserver.ServerTool {
	tool := mcplib.NewTool("get_cost_summary",
		mcplib.WithDescription("Get a global cost summary across all projects"),
	)
	return mcpserver.ServerTool{
		Tool:    tool,
		Handler: s.handleGetCostSummary,
	}
}

func (s *Server) handleListProjects(ctx context.Context, _ mcplib.CallToolRequest) (*mcplib.CallToolResult, error) { //nolint:gocritic // hugeParam: mcp-go handler signature
	if s.deps.ProjectLister == nil {
		return mcplib.NewToolResultError("project lister not configured"), nil
	}
	projects, err := s.deps.ProjectLister.ListProjects(ctx)
	if err != nil {
		return mcplib.NewToolResultErrorFromErr("failed to list projects", err), nil
	}
	data, err := json.Marshal(projects)
	if err != nil {
		return mcplib.NewToolResultErrorFromErr("failed to marshal projects", err), nil
	}
	return toolResultJSON(string(data)), nil
}

func (s *Server) handleGetProject(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) { //nolint:gocritic // hugeParam: mcp-go handler signature
	if s.deps.ProjectLister == nil {
		return mcplib.NewToolResultError("project lister not configured"), nil
	}
	args := req.GetArguments()
	projectID, ok := args["project_id"].(string)
	if !ok || projectID == "" {
		return mcplib.NewToolResultError("project_id is required"), nil
	}
	p, err := s.deps.ProjectLister.GetProject(ctx, projectID)
	if err != nil {
		return mcplib.NewToolResultErrorFromErr(
			fmt.Sprintf("failed to get project %s", projectID), err,
		), nil
	}
	data, err := json.Marshal(p)
	if err != nil {
		return mcplib.NewToolResultErrorFromErr("failed to marshal project", err), nil
	}
	return toolResultJSON(string(data)), nil
}

func (s *Server) handleGetRunStatus(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) { //nolint:gocritic // hugeParam: mcp-go handler signature
	if s.deps.RunReader == nil {
		return mcplib.NewToolResultError("run reader not configured"), nil
	}
	args := req.GetArguments()
	runID, ok := args["run_id"].(string)
	if !ok || runID == "" {
		return mcplib.NewToolResultError("run_id is required"), nil
	}
	r, err := s.deps.RunReader.GetRun(ctx, runID)
	if err != nil {
		return mcplib.NewToolResultErrorFromErr(
			fmt.Sprintf("failed to get run %s", runID), err,
		), nil
	}
	data, err := json.Marshal(r)
	if err != nil {
		return mcplib.NewToolResultErrorFromErr("failed to marshal run", err), nil
	}
	return toolResultJSON(string(data)), nil
}

func (s *Server) handleGetCostSummary(ctx context.Context, _ mcplib.CallToolRequest) (*mcplib.CallToolResult, error) { //nolint:gocritic // hugeParam: mcp-go handler signature
	if s.deps.CostReader == nil {
		return mcplib.NewToolResultError("cost reader not configured"), nil
	}
	summary, err := s.deps.CostReader.CostSummaryGlobal(ctx)
	if err != nil {
		return mcplib.NewToolResultErrorFromErr("failed to get cost summary", err), nil
	}
	data, err := json.Marshal(summary)
	if err != nil {
		return mcplib.NewToolResultErrorFromErr("failed to marshal cost summary", err), nil
	}
	return toolResultJSON(string(data)), nil
}
