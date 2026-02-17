//go:build integration

// Package integration_test runs API-level tests against a real PostgreSQL database.
// Requires: docker compose services (postgres) running.
// Run with: go test -tags=integration ./tests/integration/...
package integration_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // Register pgx driver for database/sql (needed by goose)

	cfhttp "github.com/Strob0t/CodeForge/internal/adapter/http"
	"github.com/Strob0t/CodeForge/internal/adapter/litellm"
	"github.com/Strob0t/CodeForge/internal/adapter/postgres"
	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
)

var (
	testServer *httptest.Server
	testPool   *pgxpool.Pool
)

func TestMain(m *testing.M) {
	ctx := context.Background()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://codeforge:codeforge_dev@localhost:5432/codeforge?sslmode=disable"
	}

	cfg := config.Defaults()
	cfg.Postgres.DSN = dsn

	pool, err := postgres.NewPool(ctx, cfg.Postgres)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot connect to postgres: %v\n", err)
		os.Exit(1)
	}
	testPool = pool

	// Run migrations
	if err := postgres.RunMigrations(ctx, dsn); err != nil {
		fmt.Fprintf(os.Stderr, "migrations failed: %v\n", err)
		os.Exit(1)
	}

	// Build real router with real store, stub queue/broadcaster
	store := postgres.NewStore(pool)
	queue := &stubQueue{}
	bc := &stubBroadcaster{}

	projectSvc := service.NewProjectService(store)
	taskSvc := service.NewTaskService(store, queue)
	agentSvc := service.NewAgentService(store, queue, bc)
	llmClient := litellm.NewClient("http://localhost:4000", "")

	handlers := &cfhttp.Handlers{
		Projects: projectSvc,
		Tasks:    taskSvc,
		Agents:   agentSvc,
		LiteLLM:  llmClient,
	}

	r := chi.NewRouter()

	// Liveness endpoint
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	cfhttp.MountRoutes(r, handlers)

	testServer = httptest.NewServer(r)

	// Clean test data before running
	cleanDB(pool)

	code := m.Run()

	// Cleanup
	cleanDB(pool)
	testServer.Close()
	pool.Close()

	os.Exit(code)
}

func cleanDB(pool *pgxpool.Pool) {
	ctx := context.Background()
	_, _ = pool.Exec(ctx, "DELETE FROM agent_events")
	_, _ = pool.Exec(ctx, "DELETE FROM tasks")
	_, _ = pool.Exec(ctx, "DELETE FROM agents")
	_, _ = pool.Exec(ctx, "DELETE FROM projects")
}

// --- Stubs ---

type stubQueue struct{}

func (q *stubQueue) Publish(_ context.Context, _ string, _ []byte) error { return nil }
func (q *stubQueue) Subscribe(_ context.Context, _ string, _ messagequeue.Handler) (func(), error) {
	return func() {}, nil
}
func (q *stubQueue) Drain() error      { return nil }
func (q *stubQueue) Close() error      { return nil }
func (q *stubQueue) IsConnected() bool { return true }

type stubBroadcaster struct{}

func (b *stubBroadcaster) BroadcastEvent(_ context.Context, _ string, _ any) {}
