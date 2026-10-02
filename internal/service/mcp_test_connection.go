package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	mcpprotocol "github.com/mark3labs/mcp-go/mcp"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/mcp"
	"github.com/Strob0t/CodeForge/internal/secrets"
)

// ErrStdioTestInCore: the Go Core never starts a stdio MCP server. Its
// command is agent tooling; it runs in the worker, as the tool user (KI-71),
// like every other command that runs for agents.
var ErrStdioTestInCore = fmt.Errorf("%w: stdio servers cannot be tested from the core; they run in the worker", domain.ErrValidation)

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

// TestConnection performs a real MCP handshake against the given sse or
// streamable_http server definition. It creates a client, calls Initialize and
// ListTools, then closes the connection. The whole operation is bounded by the
// configured timeout. A stdio definition is refused (ErrStdioTestInCore), and
// so is a url the outbound policy refuses (KI-100), before anything connects.
func (s *MCPService) TestConnection(ctx context.Context, def *mcp.ServerDef) (*MCPTestResult, error) {
	if err := def.Validate(); err != nil {
		return nil, err
	}
	if def.Transport == mcp.TransportStdio {
		return nil, ErrStdioTestInCore
	}
	if err := s.checkServerURL(ctx, def); err != nil {
		return nil, err
	}
	// An edited saved server may carry the redacted values it was read with.
	if err := s.keepStoredSecrets(ctx, def); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, s.limits.MCPTestTimeout)
	defer cancel()

	// One transport per test, closed with it: tests are rare admin actions,
	// so pooling connections across tests (and tenants) gains nothing, and
	// idle keep-alive connections would otherwise hold a descriptor for 90 s.
	httpClient := s.mcpHTTPClient()
	defer httpClient.CloseIdleConnections()

	client, err := s.createClient(def, httpClient)
	if err != nil {
		return &MCPTestResult{
			Success: false,
			Error:   "failed to create client: " + scrubURLSecrets(err, def),
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
			Error:   "initialize failed: " + scrubURLSecrets(err, def),
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
		result.Error = "tools/list failed: " + scrubURLSecrets(err, def)
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

// createClient builds an mcp-go Client for a remote server definition on
// httpClient (mcpHTTPClient: it checks every address it connects to). It
// never starts a process: stdio servers run only in the worker.
func (s *MCPService) createClient(def *mcp.ServerDef, httpClient *http.Client) (mcpclient.MCPClient, error) {
	switch def.Transport {
	case mcp.TransportStdio:
		return nil, ErrStdioTestInCore

	case mcp.TransportSSE:
		opts := []transport.ClientOption{transport.WithHTTPClient(httpClient)}
		if len(def.Headers) > 0 {
			opts = append(opts, transport.WithHeaders(def.Headers))
		}
		return mcpclient.NewSSEMCPClient(def.URL, opts...)

	case mcp.TransportStreamableHTTP:
		opts := []transport.StreamableHTTPCOption{transport.WithHTTPBasicClient(httpClient)}
		if len(def.Headers) > 0 {
			opts = append(opts, transport.WithHTTPHeaders(def.Headers))
		}
		return mcpclient.NewStreamableHttpClient(def.URL, opts...)

	default:
		return nil, fmt.Errorf("unsupported transport: %s", def.Transport)
	}
}

// minScrubLength is the shortest secret scrubURLSecrets replaces anywhere in
// a text: shorter ones would mangle ordinary words, and the url itself is
// already gone by then.
const minScrubLength = 4

// scrubURLSecrets returns the text of err without the secrets of def's url
// (KI-97 security review). The test connects with the stored url, and a
// *url.Error quotes the whole url (net/http strips only a userinfo
// password), so it is reported by its operation and cause only. Any other
// quote of the url, and every secret part of it (userinfo, credential query
// and fragment values, as written and decoded) or a header value, is then
// replaced as well.
func scrubURLSecrets(err error, def *mcp.ServerDef) string {
	text := err.Error()
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		text = strings.ReplaceAll(text, urlErr.Error(), urlErr.Op+": "+urlErr.Err.Error())
	}
	parts := secrets.URLFieldSecrets(def.URL)
	for _, value := range def.Headers {
		parts = append(parts, value)
	}
	sort.Slice(parts, func(i, j int) bool { return len(parts[i]) > len(parts[j]) })
	pairs := []string{def.URL, secrets.RedactURLField(def.URL, mcp.RedactedValue)}
	for _, part := range parts {
		if len(part) >= minScrubLength {
			pairs = append(pairs, part, mcp.RedactedValue)
		}
	}
	return strings.NewReplacer(pairs...).Replace(text)
}
