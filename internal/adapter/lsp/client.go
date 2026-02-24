// Package lsp provides a Language Server Protocol client that manages a single
// language server process, communicating via JSON-RPC 2.0 over stdio.
package lsp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/Strob0t/CodeForge/internal/config"
	lspDomain "github.com/Strob0t/CodeForge/internal/domain/lsp"
)

// Client manages a single language server process and provides code intelligence operations.
type Client struct {
	language  string
	config    lspDomain.LanguageServerConfig
	lspCfg    *config.LSP
	workspace string

	cmd    *exec.Cmd
	conn   *JSONRPCConn
	status lspDomain.ServerStatus
	mu     sync.Mutex

	nextID  atomic.Int64
	pending map[int]chan *JSONRPCMessage
	pendMu  sync.Mutex

	diagnostics map[string][]lspDomain.Diagnostic // URI -> diagnostics
	diagMu      sync.RWMutex

	onDiagnostic func(uri string, diags []lspDomain.Diagnostic)
	done         chan struct{} // closed when readLoop exits
}

// NewClient creates a new LSP client for the given language and workspace.
func NewClient(language string, cfg lspDomain.LanguageServerConfig, lspCfg *config.LSP, workspace string) *Client {
	return &Client{
		language:    language,
		config:      cfg,
		lspCfg:      lspCfg,
		workspace:   workspace,
		status:      lspDomain.ServerStatusStopped,
		pending:     make(map[int]chan *JSONRPCMessage),
		diagnostics: make(map[string][]lspDomain.Diagnostic),
		done:        make(chan struct{}),
	}
}

// SetDiagnosticCallback sets a callback invoked when diagnostics are received.
func (c *Client) SetDiagnosticCallback(fn func(uri string, diags []lspDomain.Diagnostic)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onDiagnostic = fn
}

// Status returns the current server status.
func (c *Client) Status() lspDomain.ServerStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.status
}

// Language returns the language this client manages.
func (c *Client) Language() string {
	return c.language
}

// PID returns the process ID of the language server, or 0 if not running.
func (c *Client) PID() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cmd != nil && c.cmd.Process != nil {
		return c.cmd.Process.Pid
	}
	return 0
}

// DiagnosticCount returns the total number of cached diagnostics.
func (c *Client) DiagnosticCount() int {
	c.diagMu.RLock()
	defer c.diagMu.RUnlock()
	count := 0
	for _, diags := range c.diagnostics {
		count += len(diags)
	}
	return count
}

// Start spawns the language server process and performs the LSP initialize handshake.
func (c *Client) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.status == lspDomain.ServerStatusReady || c.status == lspDomain.ServerStatusStarting {
		return nil
	}

	c.status = lspDomain.ServerStatusStarting

	if len(c.config.Command) == 0 {
		c.status = lspDomain.ServerStatusFailed
		return fmt.Errorf("no command configured for language %s", c.language)
	}

	// Check if the binary exists on PATH.
	if _, err := exec.LookPath(c.config.Command[0]); err != nil {
		c.status = lspDomain.ServerStatusFailed
		return fmt.Errorf("language server binary not found: %s", c.config.Command[0])
	}

	cmd := exec.CommandContext(ctx, c.config.Command[0], c.config.Command[1:]...) //nolint:gosec // command from trusted config
	cmd.Dir = c.workspace
	cmd.Stderr = os.Stderr // let server stderr pass through for debugging

	stdin, err := cmd.StdinPipe()
	if err != nil {
		c.status = lspDomain.ServerStatusFailed
		return fmt.Errorf("stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		c.status = lspDomain.ServerStatusFailed
		return fmt.Errorf("stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		c.status = lspDomain.ServerStatusFailed
		return fmt.Errorf("start process: %w", err)
	}

	c.cmd = cmd
	c.conn = NewJSONRPCConn(stdioPipe{stdin: stdin, stdout: stdout})
	c.done = make(chan struct{})

	// Start the read loop before sending initialize.
	go c.readLoop()

	// Perform LSP initialize handshake.
	if err := c.initialize(ctx); err != nil {
		c.status = lspDomain.ServerStatusFailed
		// Kill the process on failed init.
		_ = cmd.Process.Kill()
		return fmt.Errorf("initialize: %w", err)
	}

	c.status = lspDomain.ServerStatusReady
	slog.Info("lsp server started", "language", c.language, "pid", cmd.Process.Pid, "workspace", c.workspace)
	return nil
}

// Stop performs a graceful LSP shutdown (shutdown + exit) with timeout.
func (c *Client) Stop(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.status == lspDomain.ServerStatusStopped {
		return nil
	}

	slog.Info("lsp server stopping", "language", c.language)

	shutdownCtx, cancel := context.WithTimeout(ctx, c.lspCfg.ShutdownTimeout)
	defer cancel()

	// Send shutdown request.
	if c.conn != nil {
		_, err := c.call(shutdownCtx, "shutdown", nil)
		if err != nil {
			slog.Warn("lsp shutdown request failed", "language", c.language, "error", err)
		}
		// Send exit notification.
		_ = c.conn.Notify("exit", nil)
		_ = c.conn.Close()
	}

	// Wait for process to exit or kill it.
	if c.cmd != nil && c.cmd.Process != nil {
		done := make(chan error, 1)
		go func() { done <- c.cmd.Wait() }()
		select {
		case <-done:
		case <-shutdownCtx.Done():
			slog.Warn("lsp server did not exit gracefully, killing", "language", c.language)
			_ = c.cmd.Process.Kill()
		}
	}

	c.status = lspDomain.ServerStatusStopped
	c.conn = nil
	c.cmd = nil

	// Wait for readLoop to finish.
	<-c.done

	slog.Info("lsp server stopped", "language", c.language)
	return nil
}

// Definition returns go-to-definition locations for a position.
func (c *Client) Definition(ctx context.Context, uri string, pos lspDomain.Position) ([]lspDomain.Location, error) {
	params := textDocumentPositionParams(uri, pos)
	result, err := c.call(ctx, "textDocument/definition", params)
	if err != nil {
		return nil, err
	}
	return parseLocations(result)
}

// References returns all reference locations for a position.
func (c *Client) References(ctx context.Context, uri string, pos lspDomain.Position) ([]lspDomain.Location, error) {
	params := map[string]any{
		"textDocument": map[string]string{"uri": uri},
		"position":     map[string]int{"line": pos.Line, "character": pos.Character},
		"context":      map[string]bool{"includeDeclaration": true},
	}
	result, err := c.call(ctx, "textDocument/references", params)
	if err != nil {
		return nil, err
	}
	return parseLocations(result)
}

// DocumentSymbols returns document symbols for a file.
func (c *Client) DocumentSymbols(ctx context.Context, uri string) ([]lspDomain.DocumentSymbol, error) {
	params := map[string]any{
		"textDocument": map[string]string{"uri": uri},
	}
	result, err := c.call(ctx, "textDocument/documentSymbol", params)
	if err != nil {
		return nil, err
	}
	var symbols []lspDomain.DocumentSymbol
	if err := json.Unmarshal(result, &symbols); err != nil {
		return nil, fmt.Errorf("unmarshal symbols: %w", err)
	}
	return symbols, nil
}

// Hover returns hover information for a position.
func (c *Client) Hover(ctx context.Context, uri string, pos lspDomain.Position) (*lspDomain.HoverResult, error) {
	params := textDocumentPositionParams(uri, pos)
	result, err := c.call(ctx, "textDocument/hover", params)
	if err != nil {
		return nil, err
	}
	if result == nil || string(result) == "null" {
		return nil, nil
	}

	// LSP hover result has a complex "contents" field (string | MarkupContent | MarkedString[]).
	var raw struct {
		Contents json.RawMessage  `json:"contents"`
		Range    *lspDomain.Range `json:"range,omitempty"`
	}
	if err := json.Unmarshal(result, &raw); err != nil {
		return nil, fmt.Errorf("unmarshal hover: %w", err)
	}

	contents := extractHoverContents(raw.Contents)
	return &lspDomain.HoverResult{
		Contents: contents,
		Range:    raw.Range,
	}, nil
}

// Diagnostics returns cached diagnostics for a URI. If uri is empty, all diagnostics are returned.
func (c *Client) Diagnostics(uri string) []lspDomain.Diagnostic {
	c.diagMu.RLock()
	defer c.diagMu.RUnlock()

	if uri != "" {
		return c.diagnostics[uri]
	}

	var all []lspDomain.Diagnostic
	for _, diags := range c.diagnostics {
		all = append(all, diags...)
	}
	return all
}

// AllDiagnostics returns a copy of the full diagnostics map (URI -> diagnostics).
func (c *Client) AllDiagnostics() map[string][]lspDomain.Diagnostic {
	c.diagMu.RLock()
	defer c.diagMu.RUnlock()

	result := make(map[string][]lspDomain.Diagnostic, len(c.diagnostics))
	for k, v := range c.diagnostics {
		cp := make([]lspDomain.Diagnostic, len(v))
		copy(cp, v)
		result[k] = cp
	}
	return result
}

// OpenFile sends a textDocument/didOpen notification to the language server.
func (c *Client) OpenFile(ctx context.Context, uri, languageID, content string) error {
	params := map[string]any{
		"textDocument": map[string]any{
			"uri":        uri,
			"languageId": languageID,
			"version":    1,
			"text":       content,
		},
	}
	return c.conn.Notify("textDocument/didOpen", params)
}

// --- Internal methods ---

// initialize performs the LSP initialize/initialized handshake.
func (c *Client) initialize(ctx context.Context) error {
	workspaceURI := "file://" + c.workspace
	params := map[string]any{
		"processId": os.Getpid(),
		"rootUri":   workspaceURI,
		"capabilities": map[string]any{
			"textDocument": map[string]any{
				"publishDiagnostics": map[string]any{},
				"definition":         map[string]any{},
				"references":         map[string]any{},
				"documentSymbol":     map[string]any{},
				"hover":              map[string]any{},
			},
		},
	}
	if c.config.InitOpts != nil {
		params["initializationOptions"] = c.config.InitOpts
	}

	result, err := c.call(ctx, "initialize", params)
	if err != nil {
		return fmt.Errorf("initialize request: %w", err)
	}
	_ = result // We don't need the server capabilities for now.

	// Send initialized notification.
	if err := c.conn.Notify("initialized", map[string]any{}); err != nil {
		return fmt.Errorf("initialized notification: %w", err)
	}

	return nil
}

// call sends a JSON-RPC request and waits for the response.
func (c *Client) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := int(c.nextID.Add(1))
	ch := make(chan *JSONRPCMessage, 1)

	c.pendMu.Lock()
	c.pending[id] = ch
	c.pendMu.Unlock()

	defer func() {
		c.pendMu.Lock()
		delete(c.pending, id)
		c.pendMu.Unlock()
	}()

	if err := c.conn.Send(id, method, params); err != nil {
		return nil, fmt.Errorf("send %s: %w", method, err)
	}

	select {
	case msg := <-ch:
		if msg.Error != nil {
			return nil, msg.Error
		}
		return msg.Result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.done:
		return nil, fmt.Errorf("connection closed")
	}
}

// readLoop continuously reads messages from the language server.
// Responses are dispatched to pending callers; notifications are handled inline.
func (c *Client) readLoop() {
	defer close(c.done)

	for {
		msg, err := c.conn.ReadMessage()
		if err != nil {
			// Connection closed — normal during shutdown.
			return
		}

		if msg.ID != nil {
			// Response to a request we sent.
			c.pendMu.Lock()
			ch, ok := c.pending[*msg.ID]
			c.pendMu.Unlock()
			if ok {
				ch <- msg
			}
			continue
		}

		// Server notification.
		switch msg.Method {
		case "textDocument/publishDiagnostics":
			c.handlePublishDiagnostics(msg.Params)
		default:
			slog.Debug("lsp notification ignored", "method", msg.Method, "language", c.language)
		}
	}
}

// handlePublishDiagnostics processes diagnostic notifications from the server.
func (c *Client) handlePublishDiagnostics(raw json.RawMessage) {
	var params struct {
		URI         string                 `json:"uri"`
		Diagnostics []lspDomain.Diagnostic `json:"diagnostics"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		slog.Warn("lsp: failed to unmarshal diagnostics", "error", err)
		return
	}

	// Apply max diagnostics limit.
	diags := params.Diagnostics
	if c.lspCfg.MaxDiagnostics > 0 && len(diags) > c.lspCfg.MaxDiagnostics {
		diags = diags[:c.lspCfg.MaxDiagnostics]
	}

	c.diagMu.Lock()
	if len(diags) == 0 {
		delete(c.diagnostics, params.URI)
	} else {
		c.diagnostics[params.URI] = diags
	}
	c.diagMu.Unlock()

	// Invoke callback for WS broadcasting.
	c.mu.Lock()
	fn := c.onDiagnostic
	c.mu.Unlock()
	if fn != nil {
		fn(params.URI, diags)
	}
}

// --- Helpers ---

func textDocumentPositionParams(uri string, pos lspDomain.Position) map[string]any {
	return map[string]any{
		"textDocument": map[string]string{"uri": uri},
		"position":     map[string]int{"line": pos.Line, "character": pos.Character},
	}
}

func parseLocations(raw json.RawMessage) ([]lspDomain.Location, error) {
	if raw == nil || string(raw) == "null" {
		return nil, nil
	}

	// LSP definition can return Location | Location[] | LocationLink[].
	// Try array first.
	var locs []lspDomain.Location
	if err := json.Unmarshal(raw, &locs); err == nil {
		return locs, nil
	}

	// Try single location.
	var loc lspDomain.Location
	if err := json.Unmarshal(raw, &loc); err == nil {
		return []lspDomain.Location{loc}, nil
	}

	return nil, fmt.Errorf("unexpected definition result format")
}

// extractHoverContents normalizes the hover contents field to a markdown string.
func extractHoverContents(raw json.RawMessage) string {
	if raw == nil {
		return ""
	}

	// Try string directly.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}

	// Try MarkupContent {kind, value}.
	var mc struct {
		Kind  string `json:"kind"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal(raw, &mc); err == nil && mc.Value != "" {
		return mc.Value
	}

	// Try MarkedString[] or string[].
	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err == nil {
		var parts []string
		for _, item := range arr {
			var str string
			if err := json.Unmarshal(item, &str); err == nil {
				parts = append(parts, str)
				continue
			}
			var ms struct {
				Language string `json:"language"`
				Value    string `json:"value"`
			}
			if err := json.Unmarshal(item, &ms); err == nil {
				if ms.Language != "" {
					parts = append(parts, fmt.Sprintf("```%s\n%s\n```", ms.Language, ms.Value))
				} else {
					parts = append(parts, ms.Value)
				}
			}
		}
		return strings.Join(parts, "\n\n")
	}

	return string(raw)
}

// stdioPipe combines a stdin (writer) and stdout (reader) into an io.ReadWriteCloser.
type stdioPipe struct {
	stdin  io.WriteCloser
	stdout io.ReadCloser
}

func (p stdioPipe) Read(b []byte) (int, error)  { return p.stdout.Read(b) }
func (p stdioPipe) Write(b []byte) (int, error) { return p.stdin.Write(b) }
func (p stdioPipe) Close() error {
	_ = p.stdin.Close()
	return p.stdout.Close()
}
