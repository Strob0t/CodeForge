package lsp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/config"
	lspDomain "github.com/Strob0t/CodeForge/internal/domain/lsp"
)

func TestNewClient(t *testing.T) {
	t.Parallel()

	cfg := lspDomain.LanguageServerConfig{
		Command: []string{"gopls", "serve"},
	}
	lspCfg := &config.LSP{
		Enabled:         true,
		StartTimeout:    30 * time.Second,
		ShutdownTimeout: 10 * time.Second,
		MaxDiagnostics:  100,
	}

	client := NewClient("go", cfg, lspCfg, "/workspace")

	if client == nil {
		t.Fatal("NewClient() returned nil")
	}
	if client.Language() != "go" {
		t.Errorf("Language() = %q, want %q", client.Language(), "go")
	}
	if client.Status() != lspDomain.ServerStatusStopped {
		t.Errorf("Status() = %q, want %q", client.Status(), lspDomain.ServerStatusStopped)
	}
	if client.PID() != 0 {
		t.Errorf("PID() = %d, want 0 (not running)", client.PID())
	}
	if client.DiagnosticCount() != 0 {
		t.Errorf("DiagnosticCount() = %d, want 0", client.DiagnosticCount())
	}
}

func TestClientDiagnostics(t *testing.T) {
	t.Parallel()

	cfg := lspDomain.LanguageServerConfig{Command: []string{"test-server"}}
	lspCfg := &config.LSP{MaxDiagnostics: 100}
	client := NewClient("python", cfg, lspCfg, "/workspace")

	// Manually populate diagnostics for testing.
	client.diagnostics["file:///test.py"] = []lspDomain.Diagnostic{
		{
			Range:    lspDomain.Range{Start: lspDomain.Position{Line: 1, Character: 0}},
			Severity: lspDomain.SeverityError,
			Source:   "pyright",
			Message:  "undefined variable 'x'",
		},
		{
			Range:    lspDomain.Range{Start: lspDomain.Position{Line: 5, Character: 4}},
			Severity: lspDomain.SeverityWarning,
			Source:   "pyright",
			Message:  "unused import",
		},
	}
	client.diagnostics["file:///other.py"] = []lspDomain.Diagnostic{
		{
			Range:    lspDomain.Range{Start: lspDomain.Position{Line: 10, Character: 0}},
			Severity: lspDomain.SeverityInfo,
			Source:   "pyright",
			Message:  "type hint recommended",
		},
	}

	if client.DiagnosticCount() != 3 {
		t.Errorf("DiagnosticCount() = %d, want 3", client.DiagnosticCount())
	}

	// Get diagnostics for specific URI.
	diags := client.Diagnostics("file:///test.py")
	if len(diags) != 2 {
		t.Errorf("Diagnostics(test.py) len = %d, want 2", len(diags))
	}

	// Get diagnostics for unknown URI.
	diags = client.Diagnostics("file:///unknown.py")
	if len(diags) != 0 {
		t.Errorf("Diagnostics(unknown.py) len = %d, want 0", len(diags))
	}

	// Get all diagnostics.
	allDiags := client.Diagnostics("")
	if len(allDiags) != 3 {
		t.Errorf("Diagnostics(\"\") len = %d, want 3", len(allDiags))
	}
}

func TestAllDiagnostics(t *testing.T) {
	t.Parallel()

	cfg := lspDomain.LanguageServerConfig{Command: []string{"test"}}
	lspCfg := &config.LSP{MaxDiagnostics: 100}
	client := NewClient("go", cfg, lspCfg, "/workspace")

	client.diagnostics["file:///a.go"] = []lspDomain.Diagnostic{
		{Message: "error 1"},
	}

	all := client.AllDiagnostics()
	if len(all) != 1 {
		t.Fatalf("len(AllDiagnostics()) = %d, want 1", len(all))
	}

	// Verify it's a copy - modifying returned map should not affect internal state.
	all["file:///a.go"] = append(all["file:///a.go"], lspDomain.Diagnostic{Message: "extra"})
	if client.DiagnosticCount() != 1 {
		t.Errorf("DiagnosticCount() = %d, want 1 (original should not change)", client.DiagnosticCount())
	}
}

func TestSetDiagnosticCallback(t *testing.T) {
	t.Parallel()

	cfg := lspDomain.LanguageServerConfig{Command: []string{"test"}}
	lspCfg := &config.LSP{MaxDiagnostics: 100}
	client := NewClient("go", cfg, lspCfg, "/workspace")

	called := false
	client.SetDiagnosticCallback(func(_ string, _ []lspDomain.Diagnostic) {
		called = true
	})

	if client.onDiagnostic == nil {
		t.Error("onDiagnostic should be set after SetDiagnosticCallback")
	}

	// Simulate a diagnostic notification.
	diagParams, _ := json.Marshal(map[string]any{
		"uri": "file:///test.go",
		"diagnostics": []lspDomain.Diagnostic{
			{Message: "test error", Severity: lspDomain.SeverityError},
		},
	})
	client.handlePublishDiagnostics(diagParams)

	if !called {
		t.Error("diagnostic callback was not called")
	}
	if client.DiagnosticCount() != 1 {
		t.Errorf("DiagnosticCount() = %d, want 1 after publishing diagnostics", client.DiagnosticCount())
	}
}

func TestHandlePublishDiagnosticsClearEmpty(t *testing.T) {
	t.Parallel()

	cfg := lspDomain.LanguageServerConfig{Command: []string{"test"}}
	lspCfg := &config.LSP{MaxDiagnostics: 100}
	client := NewClient("go", cfg, lspCfg, "/workspace")

	// First, add diagnostics.
	client.diagnostics["file:///test.go"] = []lspDomain.Diagnostic{
		{Message: "error"},
	}

	// Publish empty diagnostics (server cleared them).
	diagParams, _ := json.Marshal(map[string]any{
		"uri":         "file:///test.go",
		"diagnostics": []lspDomain.Diagnostic{},
	})
	client.handlePublishDiagnostics(diagParams)

	if client.DiagnosticCount() != 0 {
		t.Errorf("DiagnosticCount() = %d, want 0 after clearing", client.DiagnosticCount())
	}
}

func TestHandlePublishDiagnosticsMaxLimit(t *testing.T) {
	t.Parallel()

	cfg := lspDomain.LanguageServerConfig{Command: []string{"test"}}
	lspCfg := &config.LSP{MaxDiagnostics: 2}
	client := NewClient("go", cfg, lspCfg, "/workspace")

	// Publish 5 diagnostics with max=2.
	diags := make([]lspDomain.Diagnostic, 5)
	for i := range diags {
		diags[i] = lspDomain.Diagnostic{Message: fmt.Sprintf("error %d", i)}
	}
	diagParams, _ := json.Marshal(map[string]any{
		"uri":         "file:///test.go",
		"diagnostics": diags,
	})
	client.handlePublishDiagnostics(diagParams)

	if client.DiagnosticCount() != 2 {
		t.Errorf("DiagnosticCount() = %d, want 2 (limited by MaxDiagnostics)", client.DiagnosticCount())
	}
}

// --- JSON-RPC tests ---

func TestJSONRPCErrorMessage(t *testing.T) {
	t.Parallel()

	e := &JSONRPCError{Code: -32600, Message: "Invalid Request"}
	got := e.Error()
	want := "jsonrpc error -32600: Invalid Request"
	if got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestJSONRPCMessageStructure(t *testing.T) {
	t.Parallel()

	id := 42
	msg := JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  "initialize",
		Params:  json.RawMessage(`{"rootUri":"file:///workspace"}`),
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded JSONRPCMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.JSONRPC != "2.0" {
		t.Errorf("JSONRPC = %q, want %q", decoded.JSONRPC, "2.0")
	}
	if decoded.ID == nil || *decoded.ID != 42 {
		t.Errorf("ID = %v, want 42", decoded.ID)
	}
	if decoded.Method != "initialize" {
		t.Errorf("Method = %q, want %q", decoded.Method, "initialize")
	}
}

func TestJSONRPCConnSendAndRead(t *testing.T) {
	t.Parallel()

	// Use a buffer so Send writes to it and ReadMessage reads from it.
	var buf bytes.Buffer
	rwc := &bufferRWC{buf: &buf}
	conn := NewJSONRPCConn(rwc)

	if conn == nil {
		t.Fatal("NewJSONRPCConn() returned nil")
	}

	if err := conn.Send(1, "test/method", map[string]string{"key": "value"}); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage() error = %v", err)
	}

	if msg.Method != "test/method" {
		t.Errorf("Method = %q, want %q", msg.Method, "test/method")
	}

	_ = conn.Close()
}

func TestJSONRPCConnRoundTrip(t *testing.T) {
	t.Parallel()

	// Use a buffer-based approach: write message, then read it.
	var buf bytes.Buffer
	rwc := &bufferRWC{buf: &buf}
	conn := NewJSONRPCConn(rwc)

	if err := conn.Send(1, "test/method", map[string]string{"key": "value"}); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage() error = %v", err)
	}

	if msg.JSONRPC != "2.0" {
		t.Errorf("JSONRPC = %q, want %q", msg.JSONRPC, "2.0")
	}
	if msg.ID == nil || *msg.ID != 1 {
		t.Errorf("ID = %v, want 1", msg.ID)
	}
	if msg.Method != "test/method" {
		t.Errorf("Method = %q, want %q", msg.Method, "test/method")
	}
}

func TestJSONRPCConnNotify(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	rwc := &bufferRWC{buf: &buf}
	conn := NewJSONRPCConn(rwc)

	if err := conn.Notify("exit", nil); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}

	msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage() error = %v", err)
	}

	// Notifications have no ID.
	if msg.ID != nil {
		t.Errorf("ID = %v, want nil for notification", msg.ID)
	}
	if msg.Method != "exit" {
		t.Errorf("Method = %q, want %q", msg.Method, "exit")
	}
}

func TestJSONRPCReadMessageMissingContentLength(t *testing.T) {
	t.Parallel()

	// Write a message without Content-Length header.
	raw := "Invalid-Header: foo\r\n\r\n{}"
	rwc := &bufferRWC{buf: bytes.NewBufferString(raw)}
	conn := NewJSONRPCConn(rwc)

	_, err := conn.ReadMessage()
	if err == nil {
		t.Fatal("ReadMessage() = nil error, want error for missing Content-Length")
	}
	if !strings.Contains(err.Error(), "Content-Length") {
		t.Errorf("error = %q, want it to mention Content-Length", err.Error())
	}
}

func TestParseLocations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     json.RawMessage
		wantLen int
		wantErr bool
	}{
		{
			name:    "null",
			raw:     json.RawMessage(`null`),
			wantLen: 0,
			wantErr: false,
		},
		{
			name:    "nil",
			raw:     nil,
			wantLen: 0,
			wantErr: false,
		},
		{
			name:    "single location",
			raw:     json.RawMessage(`{"uri":"file:///test.go","range":{"start":{"line":0,"character":0},"end":{"line":0,"character":5}}}`),
			wantLen: 1,
			wantErr: false,
		},
		{
			name:    "location array",
			raw:     json.RawMessage(`[{"uri":"file:///a.go","range":{"start":{"line":1,"character":0},"end":{"line":1,"character":10}}},{"uri":"file:///b.go","range":{"start":{"line":2,"character":0},"end":{"line":2,"character":5}}}]`),
			wantLen: 2,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			locs, err := parseLocations(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseLocations() error = %v, wantErr %v", err, tt.wantErr)
			}
			if len(locs) != tt.wantLen {
				t.Errorf("len(locs) = %d, want %d", len(locs), tt.wantLen)
			}
		})
	}
}

func TestExtractHoverContents(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  json.RawMessage
		want string
	}{
		{
			name: "nil",
			raw:  nil,
			want: "",
		},
		{
			name: "plain string",
			raw:  json.RawMessage(`"hello world"`),
			want: "hello world",
		},
		{
			name: "markup content",
			raw:  json.RawMessage(`{"kind":"markdown","value":"**func** main()"}`),
			want: "**func** main()",
		},
		{
			name: "string array",
			raw:  json.RawMessage(`["line1","line2"]`),
			want: "line1\n\nline2",
		},
		{
			name: "marked string array",
			raw:  json.RawMessage(`[{"language":"go","value":"func main()"}]`),
			want: "```go\nfunc main()\n```",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := extractHoverContents(tt.raw)
			if got != tt.want {
				t.Errorf("extractHoverContents() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTextDocumentPositionParams(t *testing.T) {
	t.Parallel()

	params := textDocumentPositionParams("file:///test.go", lspDomain.Position{Line: 5, Character: 10})

	td, ok := params["textDocument"].(map[string]string)
	if !ok {
		t.Fatal("textDocument is not map[string]string")
	}
	if td["uri"] != "file:///test.go" {
		t.Errorf("uri = %q, want %q", td["uri"], "file:///test.go")
	}

	pos, ok := params["position"].(map[string]int)
	if !ok {
		t.Fatal("position is not map[string]int")
	}
	if pos["line"] != 5 {
		t.Errorf("line = %d, want 5", pos["line"])
	}
	if pos["character"] != 10 {
		t.Errorf("character = %d, want 10", pos["character"])
	}
}

// --- Test helpers ---

// bufferRWC wraps a bytes.Buffer as an io.ReadWriteCloser.
type bufferRWC struct {
	buf *bytes.Buffer
}

func (b *bufferRWC) Read(p []byte) (int, error)  { return b.buf.Read(p) }
func (b *bufferRWC) Write(p []byte) (int, error) { return b.buf.Write(p) }
func (b *bufferRWC) Close() error                { return nil }
