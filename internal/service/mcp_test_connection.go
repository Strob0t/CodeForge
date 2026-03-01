package service

import (
	"context"
	"fmt"

	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	mcpprotocol "github.com/mark3labs/mcp-go/mcp"

	"github.com/Strob0t/CodeForge/internal/domain/mcp"
)

// MCPTestResult is the outcome of a connection test to an MCP server.
type MCPTestResult struct {
	Success       bool          `json:"success"`
	ServerName    string        `json:"server_name,omitempty"`
	ServerVersion string        `json:"server_version,omitempty"`
	Tools         []MCPTestTool `json:"tools,omitempty"`
	Error         string        `json:"error,omitempty"`
}

// MCPTestTool is a tool discovered during a connection test.
type MCPTestTool struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// TestConnection performs a real MCP handshake against the given server
// definition. It creates a client, calls Initialize and ListTools, then
// closes the connection. The whole operation is bounded by the configured timeout.
func (s *MCPService) TestConnection(ctx context.Context, def *mcp.ServerDef) (*MCPTestResult, error) {
	if err := def.Validate(); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, s.limits.MCPTestTimeout)
	defer cancel()

	client, err := s.createClient(def)
	if err != nil {
		return &MCPTestResult{
			Success: false,
			Error:   fmt.Sprintf("failed to create client: %v", err),
		}, nil
	}
	defer client.Close() //nolint:errcheck // best-effort cleanup

	// Initialize handshake.
	initReq := mcpprotocol.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcpprotocol.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcpprotocol.Implementation{
		Name:    "codeforge",
		Version: "1.0.0",
	}
	initResult, err := client.Initialize(ctx, initReq)
	if err != nil {
		return &MCPTestResult{
			Success: false,
			Error:   fmt.Sprintf("initialize failed: %v", err),
		}, nil
	}

	result := &MCPTestResult{
		Success:       true,
		ServerName:    initResult.ServerInfo.Name,
		ServerVersion: initResult.ServerInfo.Version,
	}

	// List tools.
	toolsResult, err := client.ListTools(ctx, mcpprotocol.ListToolsRequest{})
	if err != nil {
		// Initialize succeeded but tools/list failed — still partially successful.
		result.Error = fmt.Sprintf("tools/list failed: %v", err)
		return result, nil
	}

	for i := range toolsResult.Tools {
		result.Tools = append(result.Tools, MCPTestTool{
			Name:        toolsResult.Tools[i].Name,
			Description: toolsResult.Tools[i].Description,
		})
	}

	return result, nil
}

// createClient builds an mcp-go Client for the given server definition.
func (s *MCPService) createClient(def *mcp.ServerDef) (mcpclient.MCPClient, error) {
	switch def.Transport {
	case mcp.TransportStdio:
		env := envMapToSlice(def.Env)
		return mcpclient.NewStdioMCPClient(def.Command, env, def.Args...)

	case mcp.TransportSSE:
		var opts []transport.ClientOption
		if len(def.Headers) > 0 {
			opts = append(opts, transport.WithHeaders(def.Headers))
		}
		return mcpclient.NewSSEMCPClient(def.URL, opts...)

	case mcp.TransportStreamableHTTP:
		var opts []transport.StreamableHTTPCOption
		if len(def.Headers) > 0 {
			opts = append(opts, transport.WithHTTPHeaders(def.Headers))
		}
		return mcpclient.NewStreamableHttpClient(def.URL, opts...)

	default:
		return nil, fmt.Errorf("unsupported transport: %s", def.Transport)
	}
}

// envMapToSlice converts a map to the KEY=VALUE slice format expected by exec.Cmd.
func envMapToSlice(env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}
	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, k+"="+v)
	}
	return out
}
