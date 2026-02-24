// Package mcp defines domain types for Model Context Protocol (MCP) integration.
// These types represent MCP server definitions, tool descriptions, and lifecycle
// states in a transport-independent way for use across the service layers.
package mcp

import (
	"encoding/json"
	"fmt"

	"github.com/Strob0t/CodeForge/internal/domain"
)

// TransportType identifies the communication transport for an MCP server.
type TransportType string

const (
	TransportStdio TransportType = "stdio"
	TransportSSE   TransportType = "sse"
)

// validTransports is the set of recognized transport types.
var validTransports = map[TransportType]bool{
	TransportStdio: true,
	TransportSSE:   true,
}

// ServerStatus represents the lifecycle state of an MCP server.
type ServerStatus string

const (
	ServerStatusRegistered   ServerStatus = "registered"
	ServerStatusConnected    ServerStatus = "connected"
	ServerStatusDisconnected ServerStatus = "disconnected"
	ServerStatusError        ServerStatus = "error"
)

// ServerDef describes an MCP server configuration.
type ServerDef struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Transport   TransportType     `json:"transport"`
	Command     string            `json:"command,omitempty"`
	Args        []string          `json:"args,omitempty"`
	URL         string            `json:"url,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Enabled     bool              `json:"enabled"`
	Status      ServerStatus      `json:"status"`
}

// ServerTool describes a tool exposed by an MCP server.
type ServerTool struct {
	ServerID    string          `json:"server_id"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
}

// Validate checks that the ServerDef has all required fields and consistent
// transport-specific configuration. Returns a domain.ErrValidation-wrapped
// error on failure.
func (s *ServerDef) Validate() error {
	if s.Name == "" {
		return fmt.Errorf("%w: name is required", domain.ErrValidation)
	}

	if s.Transport == "" {
		return fmt.Errorf("%w: transport is required", domain.ErrValidation)
	}

	if !validTransports[s.Transport] {
		return fmt.Errorf("%w: invalid transport %q (must be \"stdio\" or \"sse\")", domain.ErrValidation, s.Transport)
	}

	switch s.Transport {
	case TransportStdio:
		if s.Command == "" {
			return fmt.Errorf("%w: command is required for stdio transport", domain.ErrValidation)
		}
	case TransportSSE:
		if s.URL == "" {
			return fmt.Errorf("%w: url is required for sse transport", domain.ErrValidation)
		}
	}

	return nil
}
