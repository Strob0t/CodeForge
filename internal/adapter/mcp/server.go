// Package mcp provides a stub for the Model Context Protocol server.
// This will be implemented in Phase 2 to expose tools to AI agents.
package mcp

import "log/slog"

// Server is a placeholder for the MCP server that will expose
// CodeForge tools (file edit, terminal, browser) to AI agents.
type Server struct {
	addr string
}

// NewServer creates a new MCP server stub.
func NewServer(addr string) *Server {
	return &Server{addr: addr}
}

// Start is a no-op stub. In Phase 2, this will start the MCP server.
func (s *Server) Start() error {
	slog.Info("mcp server stub: start called", "addr", s.addr)
	return nil
}

// Stop is a no-op stub. In Phase 2, this will gracefully stop the MCP server.
func (s *Server) Stop() error {
	slog.Info("mcp server stub: stop called")
	return nil
}
