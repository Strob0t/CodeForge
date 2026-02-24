package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/Strob0t/CodeForge/internal/domain/mcp"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

// SetStore sets the database store on the MCPService, enabling DB-backed
// CRUD operations. This is called during application startup once the
// store is available.
func (s *MCPService) SetStore(db database.Store) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.db = db
}

// CreateDB validates and persists a new MCP server definition to the database.
func (s *MCPService) CreateDB(ctx context.Context, srv *mcp.ServerDef) (*mcp.ServerDef, error) {
	if s.db == nil {
		return nil, fmt.Errorf("mcp service: database store not configured")
	}
	if err := srv.Validate(); err != nil {
		return nil, err
	}
	if srv.ID == "" {
		srv.ID = uuid.New().String()
	}
	if srv.Status == "" {
		srv.Status = mcp.ServerStatusRegistered
	}
	if err := s.db.CreateMCPServer(ctx, srv); err != nil {
		return nil, err
	}
	return srv, nil
}

// GetDB retrieves an MCP server by ID from the database.
func (s *MCPService) GetDB(ctx context.Context, id string) (*mcp.ServerDef, error) {
	if s.db == nil {
		return nil, fmt.Errorf("mcp service: database store not configured")
	}
	return s.db.GetMCPServer(ctx, id)
}

// ListDB returns all MCP servers for the current tenant from the database.
func (s *MCPService) ListDB(ctx context.Context) ([]mcp.ServerDef, error) {
	if s.db == nil {
		return nil, fmt.Errorf("mcp service: database store not configured")
	}
	return s.db.ListMCPServers(ctx)
}

// UpdateDB validates and updates an existing MCP server in the database.
func (s *MCPService) UpdateDB(ctx context.Context, srv *mcp.ServerDef) error {
	if s.db == nil {
		return fmt.Errorf("mcp service: database store not configured")
	}
	if err := srv.Validate(); err != nil {
		return err
	}
	return s.db.UpdateMCPServer(ctx, srv)
}

// DeleteDB removes an MCP server by ID from the database.
func (s *MCPService) DeleteDB(ctx context.Context, id string) error {
	if s.db == nil {
		return fmt.Errorf("mcp service: database store not configured")
	}
	return s.db.DeleteMCPServer(ctx, id)
}

// UpdateStatusDB updates an MCP server's status in the database.
func (s *MCPService) UpdateStatusDB(ctx context.Context, id string, status mcp.ServerStatus) error {
	if s.db == nil {
		return fmt.Errorf("mcp service: database store not configured")
	}
	return s.db.UpdateMCPServerStatus(ctx, id, status)
}

// AssignToProject links an MCP server to a project in the database.
func (s *MCPService) AssignToProject(ctx context.Context, projectID, serverID string) error {
	if s.db == nil {
		return fmt.Errorf("mcp service: database store not configured")
	}
	// Verify the server exists before assigning.
	if _, err := s.db.GetMCPServer(ctx, serverID); err != nil {
		return fmt.Errorf("assign mcp server: %w", err)
	}
	return s.db.AssignMCPServerToProject(ctx, projectID, serverID)
}

// UnassignFromProject removes the link between an MCP server and a project.
func (s *MCPService) UnassignFromProject(ctx context.Context, projectID, serverID string) error {
	if s.db == nil {
		return fmt.Errorf("mcp service: database store not configured")
	}
	return s.db.UnassignMCPServerFromProject(ctx, projectID, serverID)
}

// ListByProject returns all MCP servers assigned to a project.
func (s *MCPService) ListByProject(ctx context.Context, projectID string) ([]mcp.ServerDef, error) {
	if s.db == nil {
		return nil, fmt.Errorf("mcp service: database store not configured")
	}
	return s.db.ListMCPServersByProject(ctx, projectID)
}

// ListTools returns all cached tools for an MCP server.
func (s *MCPService) ListTools(ctx context.Context, serverID string) ([]mcp.ServerTool, error) {
	if s.db == nil {
		return nil, fmt.Errorf("mcp service: database store not configured")
	}
	return s.db.ListMCPServerTools(ctx, serverID)
}

// UpsertTools replaces all cached tools for an MCP server.
func (s *MCPService) UpsertTools(ctx context.Context, serverID string, tools []mcp.ServerTool) error {
	if s.db == nil {
		return fmt.Errorf("mcp service: database store not configured")
	}
	return s.db.UpsertMCPServerTools(ctx, serverID, tools)
}
