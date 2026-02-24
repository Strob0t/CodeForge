// Package mcp provides the MCP (Model Context Protocol) server adapter.
// It exposes CodeForge tools and resources to AI agents via the MCP protocol
// using the mcp-go SDK with Streamable HTTP transport.
package mcp

import (
	"context"
	"log/slog"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/Strob0t/CodeForge/internal/domain/cost"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/run"
)

// ServerConfig holds configuration for the MCP server.
type ServerConfig struct {
	Addr    string
	Name    string
	Version string
}

// ServerDeps holds narrow interfaces for MCP server tool handlers.
// Using narrow interfaces keeps the adapter decoupled from concrete services.
type ServerDeps struct {
	ProjectLister ProjectLister
	RunReader     RunReader
	CostReader    CostReader
}

// ProjectLister provides read-only access to projects.
type ProjectLister interface {
	ListProjects(ctx context.Context) ([]project.Project, error)
	GetProject(ctx context.Context, id string) (*project.Project, error)
}

// RunReader provides read-only access to agent runs.
type RunReader interface {
	GetRun(ctx context.Context, id string) (*run.Run, error)
}

// CostReader provides read-only access to cost summaries.
type CostReader interface {
	CostSummaryGlobal(ctx context.Context) ([]cost.ProjectSummary, error)
}

// Server wraps the mcp-go MCPServer and its Streamable HTTP transport.
type Server struct {
	cfg       ServerConfig
	deps      ServerDeps
	mcpServer *mcpserver.MCPServer
	httpSrv   *mcpserver.StreamableHTTPServer
}

// NewServer creates a new MCP server with tools and resources registered.
func NewServer(cfg ServerConfig, deps ServerDeps) *Server {
	mcpSrv := mcpserver.NewMCPServer(cfg.Name, cfg.Version)

	s := &Server{
		cfg:       cfg,
		deps:      deps,
		mcpServer: mcpSrv,
	}

	s.registerTools()
	s.registerResources()

	s.httpSrv = mcpserver.NewStreamableHTTPServer(mcpSrv,
		mcpserver.WithEndpointPath("/mcp"),
		mcpserver.WithStateLess(true),
	)

	return s
}

// Start begins listening for MCP requests on the configured address.
func (s *Server) Start() error {
	slog.Info("mcp server starting", "addr", s.cfg.Addr)
	go func() {
		if err := s.httpSrv.Start(s.cfg.Addr); err != nil {
			slog.Error("mcp server listen error", "error", err)
		}
	}()
	return nil
}

// Stop gracefully shuts down the MCP server.
func (s *Server) Stop(ctx context.Context) error {
	slog.Info("mcp server stopping")
	return s.httpSrv.Shutdown(ctx)
}

// MCPServer returns the underlying MCPServer for testing.
func (s *Server) MCPServer() *mcpserver.MCPServer {
	return s.mcpServer
}

// toolResultJSON wraps a JSON string in a CallToolResult with text content.
func toolResultJSON(v string) *mcplib.CallToolResult {
	return mcplib.NewToolResultText(v)
}
