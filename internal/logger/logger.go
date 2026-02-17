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
func New(cfg config.Logging) (*slog.Logger, Closer) {
	level := parseLevel(cfg.Level)

	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	})

	var closer Closer = nopCloser{}
	var h slog.Handler = handler
	if cfg.Async {
		async := NewAsyncHandler(handler, 10000, 4)
		h = async
		closer = async
	}

	return slog.New(h).With("service", cfg.Service), closer
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
