package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Strob0t/CodeForge/internal/middleware"
)

// serveAuditedAnonymous runs handler on a request without a user, as the
// public auth routes see it.
func serveAuditedAnonymous(store *recordingAuditStore, handler http.HandlerFunc) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", http.NoBody)
	middleware.AuditLogByHandler(store, "login", "auth")(handler).ServeHTTP(httptest.NewRecorder(), req)
}

// KI-172: a public auth route has no user in its context; the handler
// records the entry with the actor it resolved from the outcome.
func TestRecordAuditAs_WritesTheResolvedActor(t *testing.T) {
	store := &recordingAuditStore{}
	var recordErr error
	serveAuditedAnonymous(store, func(w http.ResponseWriter, r *http.Request) {
		recordErr = middleware.RecordAuditAs(r.Context(), "u1", "me@example.com", "u1", map[string]string{"result": "success"})
		w.WriteHeader(http.StatusOK)
	})

	if recordErr != nil {
		t.Fatalf("RecordAuditAs: %v", recordErr)
	}
	if len(store.entries) != 1 {
		t.Fatalf("entries = %+v, want one", store.entries)
	}
	e := store.entries[0]
	if e.Action != "login" || e.Resource != "auth" || e.AdminID != "u1" || e.ResourceID != "u1" ||
		e.AdminEmail == nil || *e.AdminEmail != "me@example.com" || string(e.Details) != `{"result":"success"}` {
		t.Fatalf("entry = %+v", e)
	}
}

// A failed login names the attempted email and the anonymous actor, whose
// ID is a valid UUID (audit_log.admin_id is NOT NULL).
func TestRecordAuditAs_AnonymousActorForAFailedLogin(t *testing.T) {
	store := &recordingAuditStore{}
	serveAuditedAnonymous(store, func(w http.ResponseWriter, r *http.Request) {
		_ = middleware.RecordAuditAs(r.Context(), middleware.AnonymousActorID, "who@example.com", "", map[string]string{"result": "failure"})
		w.WriteHeader(http.StatusUnauthorized)
	})

	if len(store.entries) != 1 {
		t.Fatalf("entries = %+v, want one", store.entries)
	}
	e := store.entries[0]
	if e.AdminID != "00000000-0000-0000-0000-000000000000" || e.AdminEmail == nil || *e.AdminEmail != "who@example.com" || e.ResourceID != "" {
		t.Fatalf("entry = %+v", e)
	}
}

// A request without a user whose handler records nothing (an invalid body)
// is not audited: there is no actor to write.
func TestAuditLogByHandler_NoUserAndNothingRecordedWritesNothing(t *testing.T) {
	store := &recordingAuditStore{}
	serveAuditedAnonymous(store, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "invalid request body", http.StatusBadRequest)
	})
	if len(store.entries) != 0 {
		t.Fatalf("entries = %+v, want none", store.entries)
	}
}

// Without an audit store (no recorder) RecordAuditAs does nothing.
func TestRecordAuditAs_WithoutAnAuditStoreDoesNothing(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", http.NoBody)
	if err := middleware.RecordAuditAs(req.Context(), "u1", "me@example.com", "u1", nil); err != nil {
		t.Fatalf("RecordAuditAs without a recorder = %v, want nil", err)
	}
}
