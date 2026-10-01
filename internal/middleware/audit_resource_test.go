package middleware_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/middleware"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

type recordingAuditStore struct{ entries []database.AuditEntry }

func (s *recordingAuditStore) InsertAuditEntry(_ context.Context, e *database.AuditEntry) error {
	s.entries = append(s.entries, *e)
	return nil
}

// KI-71 review: an audit entry names the resource it is about. The ID comes
// from the {id} URL parameter by default, from another URL parameter or from
// a JSON field of the request body otherwise; the handler still reads the
// whole body.
func TestAuditLogID_RecordsTheNamedResource(t *testing.T) {
	admin := &user.User{ID: "u1", Email: "admin@example.com", Role: user.RoleAdmin}
	long := strings.Repeat("x", 100_000)
	tests := []struct {
		name       string
		path       string
		body       string
		resourceID func(*http.Request) string
		want       string
	}{
		{name: "default: the id URL parameter", path: "/projects/p1/servers/s1", want: "p1"},
		{name: "another URL parameter", path: "/projects/p1/servers/s1", resourceID: middleware.URLParamID("serverId"), want: "s1"},
		{name: "a body field", path: "/projects/p1/servers", body: `{"server_id":"s2","note":"n"}`, resourceID: middleware.BodyFieldID("server_id"), want: "s2"},
		{name: "a body field that is no string", path: "/projects/p1/servers", body: `{"server_id":7}`, resourceID: middleware.BodyFieldID("server_id"), want: ""},
		{name: "a body too large to read twice", path: "/projects/p1/servers", body: `{"note":"` + long + `","server_id":"s3"}`, resourceID: middleware.BodyFieldID("server_id"), want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &recordingAuditStore{}
			var handlerBody string
			handler := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				data, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatalf("read body: %v", err)
				}
				handlerBody = string(data)
			})
			var audit func(http.Handler) http.Handler
			if tt.resourceID == nil {
				audit = middleware.AuditLog(store, "assign", "mcp_server")
			} else {
				audit = middleware.AuditLogID(store, "assign", "mcp_server", tt.resourceID)
			}
			r := chi.NewRouter()
			r.With(audit).Post("/projects/{id}/servers", handler)
			r.With(audit).Post("/projects/{id}/servers/{serverId}", handler)

			req := httptest.NewRequest(http.MethodPost, tt.path, strings.NewReader(tt.body))
			req = req.WithContext(middleware.ContextWithTestUser(req.Context(), admin))
			r.ServeHTTP(httptest.NewRecorder(), req)

			if len(store.entries) != 1 || store.entries[0].ResourceID != tt.want || store.entries[0].Resource != "mcp_server" {
				t.Fatalf("audit entries = %+v, want one for mcp_server %q", store.entries, tt.want)
			}
			if handlerBody != tt.body {
				t.Fatalf("handler read %d bytes, want the whole body (%d)", len(handlerBody), len(tt.body))
			}
		})
	}
}
