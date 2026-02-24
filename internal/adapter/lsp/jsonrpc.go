package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
)

// JSONRPCMessage represents a JSON-RPC 2.0 message (request, response, or notification).
type JSONRPCMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int            `json:"id,omitempty"`     // nil for notifications
	Method  string          `json:"method,omitempty"` // present for requests/notifications
	Params  json.RawMessage `json:"params,omitempty"` // request/notification params
	Result  json.RawMessage `json:"result,omitempty"` // response result
	Error   *JSONRPCError   `json:"error,omitempty"`  // response error
}

// JSONRPCError represents a JSON-RPC 2.0 error object.
type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *JSONRPCError) Error() string {
	return fmt.Sprintf("jsonrpc error %d: %s", e.Code, e.Message)
}

// JSONRPCConn wraps an io.ReadWriteCloser (typically stdin/stdout of an LSP process)
// and implements the JSON-RPC 2.0 over stdio transport with Content-Length header framing.
type JSONRPCConn struct {
	rwc    io.ReadWriteCloser
	reader *bufio.Reader
	mu     sync.Mutex // protects writes
}

// NewJSONRPCConn creates a new JSON-RPC connection over the given stream.
func NewJSONRPCConn(rwc io.ReadWriteCloser) *JSONRPCConn {
	return &JSONRPCConn{
		rwc:    rwc,
		reader: bufio.NewReaderSize(rwc, 64*1024),
	}
}

// Send sends a JSON-RPC request with the given method and params.
// Returns the request ID used.
func (c *JSONRPCConn) Send(id int, method string, params any) error {
	raw, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("marshal params: %w", err)
	}

	msg := JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  method,
		Params:  raw,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}

	return c.writeMessage(data)
}

// Notify sends a JSON-RPC notification (no ID, no response expected).
func (c *JSONRPCConn) Notify(method string, params any) error {
	raw, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("marshal params: %w", err)
	}

	msg := JSONRPCMessage{
		JSONRPC: "2.0",
		Method:  method,
		Params:  raw,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}

	return c.writeMessage(data)
}

// ReadMessage reads one JSON-RPC message from the connection.
// Blocks until a full message is available or the connection is closed.
func (c *JSONRPCConn) ReadMessage() (*JSONRPCMessage, error) {
	data, err := c.readMessage()
	if err != nil {
		return nil, err
	}

	var msg JSONRPCMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("unmarshal message: %w", err)
	}

	return &msg, nil
}

// Close closes the underlying connection.
func (c *JSONRPCConn) Close() error {
	return c.rwc.Close()
}

// writeMessage writes a JSON-RPC message with Content-Length header framing.
func (c *JSONRPCConn) writeMessage(data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	if _, err := io.WriteString(c.rwc, header); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	if _, err := c.rwc.Write(data); err != nil {
		return fmt.Errorf("write body: %w", err)
	}
	return nil
}

// readMessage reads one Content-Length-framed message from the connection.
func (c *JSONRPCConn) readMessage() ([]byte, error) {
	// Read headers until empty line.
	contentLength := -1
	for {
		line, err := c.reader.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("read header: %w", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break // End of headers
		}
		if strings.HasPrefix(line, "Content-Length: ") {
			val := strings.TrimPrefix(line, "Content-Length: ")
			n, err := strconv.Atoi(val)
			if err != nil {
				return nil, fmt.Errorf("parse Content-Length %q: %w", val, err)
			}
			contentLength = n
		}
		// Ignore other headers (e.g. Content-Type).
	}

	if contentLength < 0 {
		return nil, fmt.Errorf("missing Content-Length header")
	}

	// Read exactly contentLength bytes.
	body := make([]byte, contentLength)
	if _, err := io.ReadFull(c.reader, body); err != nil {
		return nil, fmt.Errorf("read body (%d bytes): %w", contentLength, err)
	}

	return body, nil
}
