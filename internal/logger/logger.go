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
func New(cfg config.Logging) *slog.Logger {
	level := parseLevel(cfg.Level)

	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	})

	return slog.New(handler).With("service", cfg.Service)
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
