package middleware_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/middleware"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

// recordingAuditStore refuses what PostgreSQL refuses: a NUL byte in text
// or JSONB.
type recordingAuditStore struct {
	entries []database.AuditEntry
	err     error
}

func (s *recordingAuditStore) InsertAuditEntry(_ context.Context, e *database.AuditEntry) error {
	if s.err != nil {
		return s.err
	}
	if strings.ContainsRune(e.ResourceID, 0) || strings.Contains(string(e.Details), `\u0000`) {
		return errors.New("invalid byte sequence for encoding UTF8: 0x00")
	}
	s.entries = append(s.entries, *e)
	return nil
}

func serveAudited(store *recordingAuditStore, handler http.HandlerFunc) {
	admin := &user.User{ID: "u1", Email: "admin@example.com", Role: user.RoleAdmin}
	req := httptest.NewRequest(http.MethodPost, "/", http.NoBody)
	req = req.WithContext(middleware.ContextWithTestUser(req.Context(), admin))
	middleware.AuditLogByHandler(store, "assign", "mcp_server")(handler).ServeHTTP(httptest.NewRecorder(), req)
}

// KI-71 review: the handler names the audited resource from what it decoded;
// the entry merges what it knew before (AuditContext) and is written before
// the change, storable whatever the requester sent.
func TestRecordAudit_WritesTheHandlersResource(t *testing.T) {
	store := &recordingAuditStore{}
	var recordErr error
	serveAudited(store, func(w http.ResponseWriter, r *http.Request) {
		middleware.AuditContext(r.Context(), map[string]string{"project_id": "p\x001"})
		recordErr = middleware.RecordAudit(r.Context(), "s1\x00\xffdecoy", map[string]string{"note": "n"})
		w.WriteHeader(http.StatusNoContent)
	})

	if recordErr != nil {
		t.Fatalf("RecordAudit: %v", recordErr)
	}
	if len(store.entries) != 1 {
		t.Fatalf("entries = %+v, want one", store.entries)
	}
	e := store.entries[0]
	if e.Action != "assign" || e.Resource != "mcp_server" || e.ResourceID != "s1��decoy" || e.AdminID != "u1" {
		t.Fatalf("entry = %+v", e)
	}
	if got := string(e.Details); got != `{"note":"n","project_id":"p`+"�"+`1"}` {
		t.Fatalf("details = %s", got)
	}
}

func TestRecordAudit_AFailedWriteRefusesTheChange(t *testing.T) {
	store := &recordingAuditStore{err: errors.New("audit_log unavailable")}
	var recordErr error
	serveAudited(store, func(_ http.ResponseWriter, r *http.Request) {
		recordErr = middleware.RecordAudit(r.Context(), "s1", nil)
	})

	if !errors.Is(recordErr, middleware.ErrAuditUnavailable) {
		t.Fatalf("RecordAudit = %v, want ErrAuditUnavailable", recordErr)
	}
	if len(store.entries) != 0 {
		t.Fatalf("entries = %+v, want none (and no second attempt after the handler)", store.entries)
	}
}

func TestAuditLogByHandler_ARequestRefusedBeforeRecordingIsAudited(t *testing.T) {
	store := &recordingAuditStore{}
	serveAudited(store, func(w http.ResponseWriter, r *http.Request) {
		middleware.AuditContext(r.Context(), map[string]string{"project_id": "p1"})
		http.Error(w, "invalid request body", http.StatusBadRequest)
	})

	if len(store.entries) != 1 || store.entries[0].ResourceID != "" ||
		string(store.entries[0].Details) != `{"project_id":"p1","status":"400"}` {
		t.Fatalf("entries = %+v, want one with the project and the status", store.entries)
	}
}

func TestRecordAudit_WithoutAnAuditStoreDoesNothing(t *testing.T) {
	if err := middleware.RecordAudit(context.Background(), "s1", nil); err != nil {
		t.Fatalf("RecordAudit without a recorder = %v, want nil", err)
	}
	middleware.AuditContext(context.Background(), map[string]string{"project_id": "p1"})
}
