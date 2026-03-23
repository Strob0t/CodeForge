package middleware

import (
	"context"
	"log/slog"
	"net"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/Strob0t/CodeForge/internal/port/database"
)

// AuditStore is the minimal interface needed by the audit middleware.
type AuditStore interface {
	InsertAuditEntry(ctx context.Context, e *database.AuditEntry) error
}

// AuditLog returns middleware that writes an immutable audit trail entry
// for every request that reaches the wrapped handler.
// The admin identity is read from the request context (set by Auth middleware).
func AuditLog(store AuditStore, action, resource string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u := UserFromContext(r.Context())
			if u != nil {
				resourceID := chi.URLParam(r, "id")
				ip, _, _ := net.SplitHostPort(r.RemoteAddr)
				if ip == "" {
					ip = r.RemoteAddr
				}
				entry := &database.AuditEntry{
					AdminID:    u.ID,
					AdminEmail: u.Email,
					Action:     action,
					Resource:   resource,
					ResourceID: resourceID,
					IPAddress:  ip,
				}
				if err := store.InsertAuditEntry(r.Context(), entry); err != nil {
					slog.Error("audit log write failed",
						"action", action,
						"resource", resource,
						"admin_id", u.ID,
						"error", err,
					)
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
