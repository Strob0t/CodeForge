package main

import (
	"context"
	"net/http"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	cfhttp "github.com/Strob0t/CodeForge/internal/adapter/http"
	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

type auditStoreStub struct{}

func (auditStoreStub) InsertAuditEntry(context.Context, *database.AuditEntry) error { return nil }
func (auditStoreStub) ListAuditEntries(context.Context, string, int, int) ([]database.AuditEntry, error) {
	return nil, nil
}

// KI-172: the API routes are mounted with the audit store, so the audit
// middleware writes entries and GET /audit-logs exists in production.
func TestAPIRouteOptions_WireTheAuditStore(t *testing.T) {
	store := auditStoreStub{}
	h := &cfhttp.Handlers{Limits: &config.Limits{}}
	h.WireGroups()
	r := chi.NewRouter()
	cfhttp.MountRoutes(r, h, apiRouteOptions(nil, store)...)

	var auditLogsMounted, deleteProjectAudited bool
	err := chi.Walk(r, func(method, route string, _ http.Handler, mws ...func(http.Handler) http.Handler) error {
		switch {
		case method == http.MethodGet && route == "/api/v1/audit-logs":
			auditLogsMounted = true
		case method == http.MethodDelete && route == "/api/v1/projects/{id}":
			for _, mw := range mws {
				// The closure of middleware.AuditLog, whether inlined into
				// the route mounting ("...MountRoutes.func1.AuditLog.3") or not.
				if strings.Contains(runtime.FuncForPC(reflect.ValueOf(mw).Pointer()).Name(), ".AuditLog.") {
					deleteProjectAudited = true
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !auditLogsMounted {
		t.Error("GET /api/v1/audit-logs is not mounted: the audit store is not wired")
	}
	if !deleteProjectAudited {
		t.Error("DELETE /api/v1/projects/{id} has no audit middleware: the audit store is not wired")
	}
}
