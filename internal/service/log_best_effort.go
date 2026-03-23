package service

import (
	"context"
	"log/slog"
)

// logBestEffort logs a warning if err is non-nil. Use for best-effort
// operations (state updates, metric recording) that should not block
// the main flow but must never be silently discarded.
func logBestEffort(ctx context.Context, err error, op string, attrs ...slog.Attr) {
	if err != nil {
		attrs = append(attrs, slog.String("operation", op), slog.Any("error", err))
		slog.LogAttrs(ctx, slog.LevelWarn, "best-effort operation failed", attrs...)
	}
}
