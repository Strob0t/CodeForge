package database

import (
	"context"

	"github.com/Strob0t/CodeForge/internal/domain/mcp"
)

// MCPStore defines database operations for MCP server management.
type MCPStore interface {
	CreateMCPServer(ctx context.Context, s *mcp.ServerDef) error
	GetMCPServer(ctx context.Context, id string) (*mcp.ServerDef, error)
	ListMCPServers(ctx context.Context) ([]mcp.ServerDef, error)
	UpdateMCPServer(ctx context.Context, s *mcp.ServerDef) error
	DeleteMCPServer(ctx context.Context, id string) error
	UpdateMCPServerStatus(ctx context.Context, id string, status mcp.ServerStatus) error
	AssignMCPServerToProject(ctx context.Context, projectID, serverID string) error
	UnassignMCPServerFromProject(ctx context.Context, projectID, serverID string) error
	ListMCPServersByProject(ctx context.Context, projectID string) ([]mcp.ServerDef, error)
	UpsertMCPServerTools(ctx context.Context, serverID string, tools []mcp.ServerTool) error
	ListMCPServerTools(ctx context.Context, serverID string) ([]mcp.ServerTool, error)
}
