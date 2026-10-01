package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
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
// The entry names the resource of the {id} URL parameter.
func AuditLog(store AuditStore, action, resource string) func(http.Handler) http.Handler {
	return AuditLogID(store, action, resource, URLParamID("id"))
}

// URLParamID names the audited resource by a URL parameter.
func URLParamID(param string) func(*http.Request) string {
	return func(r *http.Request) string { return chi.URLParam(r, param) }
}

// auditBodyPeekLimit is how much of a request body BodyFieldID reads to find
// the resource ID; the handler still reads the whole body.
const auditBodyPeekLimit = 64 << 10

// BodyFieldID names the audited resource by a string field of the JSON
// request body ("" when the body is larger than 64 KiB, not JSON, or the
// field is no string). The handler reads the body unchanged.
func BodyFieldID(field string) func(*http.Request) string {
	return func(r *http.Request) string {
		if r.Body == nil {
			return ""
		}
		peeked, err := io.ReadAll(io.LimitReader(r.Body, auditBodyPeekLimit+1))
		r.Body = readCloser{Reader: io.MultiReader(bytes.NewReader(peeked), r.Body), Closer: r.Body}
		if err != nil || len(peeked) > auditBodyPeekLimit {
			return ""
		}
		var fields map[string]json.RawMessage
		var id string
		if json.Unmarshal(peeked, &fields) != nil || json.Unmarshal(fields[field], &id) != nil {
			return ""
		}
		return id
	}
}

type readCloser struct {
	io.Reader
	io.Closer
}

// AuditLogID is AuditLog naming the audited resource by resourceID.
func AuditLogID(store AuditStore, action, resource string, resourceID func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u := UserFromContext(r.Context())
			if u != nil {
				resourceID := resourceID(r)
				ip, _, _ := net.SplitHostPort(r.RemoteAddr)
				if ip == "" {
					ip = r.RemoteAddr
				}
				email := u.Email
				entry := &database.AuditEntry{
					AdminID:    u.ID,
					AdminEmail: &email,
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
