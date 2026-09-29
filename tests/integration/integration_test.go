//go:build integration

// Package integration_test runs API-level tests against a real PostgreSQL database.
// Requires: docker compose services (postgres) running.
// Run with: go test -tags=integration ./tests/integration/...
//
// The database may be shared with other test runs, so tests never assume empty
// tables: tests that count rows work in a fresh tenant (see newTestTenant) and
// remove only the rows of that tenant afterwards.
package integration_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // Register pgx driver for database/sql (needed by goose)

	cfhttp "github.com/Strob0t/CodeForge/internal/adapter/http"
	"github.com/Strob0t/CodeForge/internal/adapter/litellm"
	"github.com/Strob0t/CodeForge/internal/adapter/postgres"
	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/tenant"
	"github.com/Strob0t/CodeForge/internal/middleware"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
)

var (
	// testServer runs with authentication disabled (the default deployment):
	// every request acts as the default admin in the default tenant.
	testServer *httptest.Server
	// testAuthServer runs with authentication enabled.
	testAuthServer *httptest.Server
	testStore      *postgres.Store
	testPool       *pgxpool.Pool
)

// noRedirectClient returns redirects to the test instead of following them.
var noRedirectClient = &http.Client{
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

func testDSN() string {
	if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
		return dsn
	}
	return "postgres://codeforge:codeforge_dev@localhost:5432/codeforge?sslmode=disable"
}

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	ctx := context.Background()
	dsn := testDSN()

	cfg := config.Defaults()
	cfg.Postgres.DSN = dsn

	pool, err := postgres.NewPool(ctx, cfg.Postgres)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot connect to postgres: %v\n", err)
		return 1
	}
	defer pool.Close()
	testPool = pool

	if err := postgres.RunMigrations(ctx, dsn); err != nil {
		fmt.Fprintf(os.Stderr, "migrations failed: %v\n", err)
		return 1
	}

	// Real router with real store, stub queue/broadcaster.
	store := postgres.NewStore(pool)
	testStore = store
	queue := &stubQueue{}
	authCfg := config.Auth{
		Enabled:            true,
		JWTSecret:          "integration-test-secret-must-be-long",
		AccessTokenExpiry:  15 * time.Minute,
		RefreshTokenExpiry: 7 * 24 * time.Hour,
		BcryptCost:         4, // low cost for fast tests
	}
	authSvc := service.NewAuthService(store, &authCfg)

	handlers := &cfhttp.Handlers{
		Projects: service.NewProjectService(store, ""),
		Tasks:    service.NewTaskService(store, queue),
		Agents:   service.NewAgentService(store, queue, &stubBroadcaster{}),
		LLM:      litellm.NewClient("http://localhost:4000", ""),
		Auth:     authSvc,
		Limits:   &cfg.Limits,
	}
	handlers.WireGroups()

	testServer = httptest.NewServer(newRouter(handlers, authSvc, false))
	defer testServer.Close()
	testAuthServer = httptest.NewServer(newRouter(handlers, authSvc, true))
	defer testAuthServer.Close()

	return m.Run()
}

// newRouter mirrors the middleware order of cmd/codeforge/main.go that the
// API depends on: Auth (a default admin when disabled), then TenantID.
func newRouter(h *cfhttp.Handlers, authSvc *service.AuthService, authEnabled bool) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Auth(authSvc, authEnabled))
	r.Use(middleware.TenantID)
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	cfhttp.MountRoutes(r, h, config.Webhook{})
	return r
}

// newTestTenant creates a tenant that only the calling test uses and deletes
// the rows created in it when the test ends.
func newTestTenant(t *testing.T) string {
	t.Helper()
	tn, err := testStore.CreateTenant(context.Background(), tenant.CreateRequest{
		Name: "Integration Test Tenant",
		Slug: "integ-" + uuid.NewString()[:8],
	})
	if err != nil {
		t.Fatalf("create test tenant: %v", err)
	}
	t.Cleanup(func() { deleteTenant(t, tn.ID) })
	return tn.ID
}

// deleteTenant removes a test tenant with its projects (cascading to tasks
// and agents) and users (cascading to tokens and API keys).
func deleteTenant(t *testing.T, tenantID string) {
	t.Helper()
	ctx := context.Background()
	for _, stmt := range []string{
		"DELETE FROM projects WHERE tenant_id = $1",
		"DELETE FROM users WHERE tenant_id = $1",
		"DELETE FROM tenants WHERE id = $1",
	} {
		if _, err := testPool.Exec(ctx, stmt, tenantID); err != nil {
			t.Errorf("cleanup tenant %s: %s: %v", tenantID, stmt, err)
		}
	}
}

// tenantRequest builds a JSON request against the auth-enabled server for the
// given tenant. Public endpoints (setup, login, password reset) resolve the
// tenant from the X-Tenant-ID header; authenticated requests use the token's
// tenant. An empty token sends no Authorization header.
func tenantRequest(t *testing.T, method, path, tenantID, token string, body []byte) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), method, testAuthServer.URL+path, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build %s %s: %v", method, path, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant-ID", tenantID)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req
}

// doRequest sends req and closes the response body when the test ends.
func doRequest(t *testing.T, req *http.Request) *http.Response {
	t.Helper()
	resp, err := noRedirectClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", req.Method, req.URL.Path, err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

// --- Stubs ---

type stubQueue struct{}

func (q *stubQueue) Publish(_ context.Context, _ string, _ []byte) error { return nil }
func (q *stubQueue) PublishWithDedup(_ context.Context, _ string, _ []byte, _ string) error {
	return nil
}
func (q *stubQueue) Subscribe(_ context.Context, _ string, _ messagequeue.Handler) (func(), error) {
	return func() {}, nil
}
func (q *stubQueue) Drain() error      { return nil }
func (q *stubQueue) Close() error      { return nil }
func (q *stubQueue) IsConnected() bool { return true }

type stubBroadcaster struct{}

func (b *stubBroadcaster) BroadcastEvent(_ context.Context, _ string, _ any) {}
