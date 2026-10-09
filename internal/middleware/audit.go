package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/Strob0t/CodeForge/internal/domain/user"
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
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if u := UserFromContext(r.Context()); u != nil {
				entry := newAuditEntry(u, action, resource, r)
				entry.ResourceID = chi.URLParam(r, "id")
				if err := store.InsertAuditEntry(r.Context(), entry); err != nil {
					logAuditFailure(r.Context(), entry, err)
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ErrAuditUnavailable is returned by RecordAudit when the entry could not be
// written; the caller must not make the change.
var ErrAuditUnavailable = errors.New("audit log unavailable")

type auditRecorderKey struct{}

// auditRecorder is the audit entry of one request whose handler names the
// audited resource itself (RecordAudit).
type auditRecorder struct {
	store    AuditStore
	entry    *database.AuditEntry
	known    map[string]string // details known before decoding (AuditContext)
	recorded bool
}

// AuditLogByHandler returns audit middleware for actions whose handler names
// the audited resource from what it decoded and acts on (RecordAudit), never
// from a second reading of the request: a requester could make that differ
// (trailing data, case-insensitive duplicate keys, padding). A request the
// handler refused before recording is audited after it, with its status.
// A request without a user (the public auth routes, KI-172) is audited only
// when its handler records it with the actor it resolved (RecordAuditAs).
func AuditLogByHandler(store AuditStore, action, resource string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u := UserFromContext(r.Context())
			rec := &auditRecorder{store: store, entry: newAuditEntry(u, action, resource, r)}
			sw := &statusWriter{ResponseWriter: w}
			next.ServeHTTP(sw, r.WithContext(context.WithValue(r.Context(), auditRecorderKey{}, rec)))
			if rec.recorded || u == nil {
				return
			}
			rec.entry.Details = auditDetails(rec.known, map[string]string{"status": strconv.Itoa(sw.status())})
			if err := store.InsertAuditEntry(r.Context(), rec.entry); err != nil {
				logAuditFailure(r.Context(), rec.entry, err)
			}
		})
	}
}

// AuditContext gives the request's audit entry details known before the
// handler decodes anything (e.g. the project of the URL); they are recorded
// with the entry whether or not the handler gets to RecordAudit.
func AuditContext(ctx context.Context, details map[string]string) {
	if rec, ok := ctx.Value(auditRecorderKey{}).(*auditRecorder); ok {
		rec.known = details
	}
}

// RecordAudit writes the request's audit entry for resourceID with details
// (AuditLogByHandler). Call it with the values the handler decoded and acts
// on, after validating them and before changing anything: an error
// (ErrAuditUnavailable) means no entry was written, and the change must not
// be made. A request without an audit store does nothing.
func RecordAudit(ctx context.Context, resourceID string, details map[string]string) error {
	rec, ok := ctx.Value(auditRecorderKey{}).(*auditRecorder)
	if !ok {
		return nil
	}
	return rec.record(ctx, rec.entry.AdminID, rec.entry.AdminEmail, resourceID, details)
}

// AnonymousActorID is the actor of an audit entry whose request resolved no
// user (a failed login, a password reset request): a reserved UUID of its
// own (the entry's admin_id must be a UUID), not the identity every request
// acts as while authentication is disabled.
const AnonymousActorID = user.AnonymousActorUserID

// RecordAuditAs is RecordAudit for a request without a user in its context
// (the public auth routes, KI-172): the handler names the actor it resolved
// from the outcome, the user on success or AnonymousActorID with the
// attempted email on a failure. The details never hold a credential.
func RecordAuditAs(ctx context.Context, actorID, actorEmail, resourceID string, details map[string]string) error {
	rec, ok := ctx.Value(auditRecorderKey{}).(*auditRecorder)
	if !ok {
		return nil
	}
	email := auditText(actorEmail)
	return rec.record(ctx, auditText(actorID), &email, resourceID, details)
}

// record writes one entry for resourceID with the actor and details. The
// request's entry is copied, so a handler that acts on several resources
// (a batch) records each with its own call.
func (rec *auditRecorder) record(ctx context.Context, actorID string, actorEmail *string, resourceID string, details map[string]string) error {
	rec.recorded = true
	entry := *rec.entry
	entry.AdminID = actorID
	entry.AdminEmail = actorEmail
	entry.ResourceID = auditText(resourceID)
	entry.Details = auditDetails(rec.known, details)
	if err := rec.store.InsertAuditEntry(ctx, &entry); err != nil {
		logAuditFailure(ctx, &entry, err)
		return errors.Join(ErrAuditUnavailable, err)
	}
	return nil
}

// newAuditEntry starts the entry of a request by u, which is nil on the
// public auth routes until the handler resolves the actor (RecordAuditAs).
func newAuditEntry(u *user.User, action, resource string, r *http.Request) *database.AuditEntry {
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	if ip == "" {
		ip = r.RemoteAddr
	}
	entry := &database.AuditEntry{
		Action:    action,
		Resource:  resource,
		IPAddress: ip,
	}
	if u != nil {
		email := u.Email
		entry.AdminID = u.ID
		entry.AdminEmail = &email
	}
	return entry
}

// auditText makes a requester's value storable: PostgreSQL text holds no NUL
// byte and no invalid UTF-8, and an entry that cannot be inserted is lost.
func auditText(s string) string {
	return strings.ReplaceAll(strings.ToValidUTF8(s, "�"), "\x00", "�")
}

// auditDetails encodes the merged details as JSON with storable values
// (JSONB holds no \u0000 either).
func auditDetails(parts ...map[string]string) []byte {
	safe := map[string]string{}
	for _, details := range parts {
		for k, v := range details {
			safe[auditText(k)] = auditText(v)
		}
	}
	if len(safe) == 0 {
		return nil
	}
	data, err := json.Marshal(safe)
	if err != nil {
		return nil
	}
	return data
}

// logAuditFailure keeps a lost entry in the log, with everything it held.
func logAuditFailure(ctx context.Context, e *database.AuditEntry, err error) {
	slog.ErrorContext(ctx, "audit log write failed",
		"action", e.Action,
		"resource", e.Resource,
		"resource_id", e.ResourceID,
		"details", string(e.Details),
		"admin_id", e.AdminID,
		"error", err,
	)
}

// statusWriter records the status of a response.
type statusWriter struct {
	http.ResponseWriter
	code int
}

func (w *statusWriter) WriteHeader(code int) {
	if w.code == 0 {
		w.code = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.code == 0 {
		w.code = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

func (w *statusWriter) status() int {
	if w.code == 0 {
		return http.StatusOK
	}
	return w.code
}
