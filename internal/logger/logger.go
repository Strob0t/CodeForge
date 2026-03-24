// Package logger provides structured logging setup for CodeForge.
package logger

import (
	"log/slog"
	"os"
	"strings"

	"github.com/Strob0t/CodeForge/internal/config"
)

// New creates a *slog.Logger from the given Logging config.
// Output is JSON to stdout with a "service" attribute on every record.
// When cfg.Async is true the handler writes via a buffered channel;
// the caller must call Closer.Close() on shutdown to flush remaining records.
// The returned DroppedCounter reports how many log records were dropped
// (always 0 in synchronous mode).
func New(cfg config.Logging) (*slog.Logger, Closer, DroppedCounter) {
	level := parseLevel(cfg.Level)

	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	})

	// Wrap with PII/secret redaction before any buffering so secrets
	// never reach the output stream regardless of async mode.
	redacted := NewRedactHandler(handler)

	var closer Closer = nopCloser{}
	var dropped DroppedCounter = nopDroppedCounter{}
	var h slog.Handler = redacted
	if cfg.Async {
		async := NewAsyncHandler(redacted, 10000, 4)
		h = async
		closer = async
		dropped = async
	}

	return slog.New(h).With("service", cfg.Service), closer, dropped
}

// parseLevel converts a string log level to slog.Level.
func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
