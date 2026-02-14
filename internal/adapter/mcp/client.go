package mcp

import "log/slog"

// Client is a placeholder for an MCP client that connects to external
// MCP-compatible tools (e.g., filesystem, databases, APIs).
type Client struct {
	name string
}

// NewClient creates a new MCP client stub.
func NewClient(name string) *Client {
	return &Client{name: name}
}

// Connect is a no-op stub. In Phase 2, this will establish the MCP connection.
func (c *Client) Connect() error {
	slog.Info("mcp client stub: connect called", "name", c.name)
	return nil
}

// Close is a no-op stub.
func (c *Client) Close() error {
	slog.Info("mcp client stub: close called", "name", c.name)
	return nil
}
