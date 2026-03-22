// Package mcp provides the MCP (Model Context Protocol) server adapter.
// It exposes CodeForge tools and resources to AI agents via the MCP protocol
// using the mcp-go SDK with Streamable HTTP transport.
package mcp

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/Strob0t/CodeForge/internal/domain/cost"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// ServerConfig holds configuration for the MCP server.
type ServerConfig struct {
	Addr    string
	Name    string
	Version string
	APIKey  string // If non-empty, requests must provide this key via Authorization header.
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
// It panics if any required dependency is nil (fail-fast at construction time).
func NewServer(cfg ServerConfig, deps ServerDeps) *Server {
	if deps.ProjectLister == nil {
		panic("MCP ServerDeps.ProjectLister must not be nil")
	}
	if deps.RunReader == nil {
		panic("MCP ServerDeps.RunReader must not be nil")
	}
	if deps.CostReader == nil {
		panic("MCP ServerDeps.CostReader must not be nil")
	}

	mcpSrv := mcpserver.NewMCPServer(cfg.Name, cfg.Version)

	s := &Server{
		cfg:       cfg,
		deps:      deps,
		mcpServer: mcpSrv,
	}

	s.registerTools()
	s.registerResources()

	// Build StreamableHTTPServer options.
	opts := []mcpserver.StreamableHTTPOption{
		mcpserver.WithEndpointPath("/mcp"),
		mcpserver.WithStateLess(true),
	}

	// Wire AuthMiddleware with tenant context injection.
	// MCP currently uses a shared API key (not per-tenant), so we inject
	// the default tenant ID for all requests. When per-tenant MCP auth
	// is added, this should resolve the tenant from the API key.
	{
		mux := http.NewServeMux()
		tmpHTTP := mcpserver.NewStreamableHTTPServer(mcpSrv,
			mcpserver.WithEndpointPath("/mcp"),
			mcpserver.WithStateLess(true),
		)
		mux.Handle("/mcp", AuthMiddleware(cfg.APIKey, tenantctx.DefaultTenantID, tmpHTTP))
		httpSrv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second} //nolint:gosec // timeout set
		opts = append(opts, mcpserver.WithStreamableHTTPServer(httpSrv))
	}

	s.httpSrv = mcpserver.NewStreamableHTTPServer(mcpSrv, opts...)

	return s
}

// Start begins listening for MCP requests on the configured address.
// It waits briefly for the listener to bind, returning any immediate
// errors (e.g. port already in use) instead of swallowing them.
func (s *Server) Start() error {
	slog.Info("mcp server starting", "addr", s.cfg.Addr)
	errCh := make(chan error, 1)
	go func() {
		errCh <- s.httpSrv.Start(s.cfg.Addr)
	}()
	// Wait briefly for immediate listen errors (port conflict, permission denied).
	select {
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("mcp server listen: %w", err)
		}
		return nil // server exited without error (unlikely but safe)
	case <-time.After(100 * time.Millisecond):
		// Listener bound successfully; server continues in background.
		// Drain errors asynchronously so the goroutine is not leaked.
		go func() {
			if err := <-errCh; err != nil {
				slog.Error("mcp server listen error", "error", err)
			}
		}()
		return nil
	}
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
