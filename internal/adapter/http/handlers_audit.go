package http

import (
	"context"
	"net/http"

	"github.com/Strob0t/CodeForge/internal/port/database"
)

// auditLogReader is the minimal interface needed by the audit log handler.
type auditLogReader interface {
	ListAuditEntries(ctx context.Context, action string, limit, offset int) ([]database.AuditEntry, error)
}

// ListAuditLogs handles GET /api/v1/audit-logs?action=...&limit=...&offset=...
// Returns a paginated list of admin audit log entries (admin-only).
func ListAuditLogs(store auditLogReader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		action := r.URL.Query().Get("action")
		limit, offset := parsePagination(r, 100)

		entries, err := store.ListAuditEntries(r.Context(), action, limit, offset)
		if err != nil {
			writeInternalError(w, err)
			return
		}
		if entries == nil {
			entries = []database.AuditEntry{}
		}
		writeJSON(w, http.StatusOK, entries)
	}
}
