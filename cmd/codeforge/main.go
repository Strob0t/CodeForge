package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/Strob0t/CodeForge/internal/adapter/aider"
	cfhttp "github.com/Strob0t/CodeForge/internal/adapter/http"
	"github.com/Strob0t/CodeForge/internal/adapter/litellm"
	cfnats "github.com/Strob0t/CodeForge/internal/adapter/nats"
	"github.com/Strob0t/CodeForge/internal/adapter/postgres"
	"github.com/Strob0t/CodeForge/internal/adapter/ws"
	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/service"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	slog.Info("config loaded",
		"port", cfg.Server.Port,
		"log_level", cfg.Logging.Level,
		"pg_max_conns", cfg.Postgres.MaxConns,
	)

	ctx := context.Background()

	// --- Infrastructure ---

	// PostgreSQL
	pool, err := postgres.NewPool(ctx, cfg.Postgres.DSN)
	if err != nil {
		return fmt.Errorf("postgres: %w", err)
	}
	defer pool.Close()
	slog.Info("postgres connected")

	// Run migrations
	if err := postgres.RunMigrations(ctx, cfg.Postgres.DSN); err != nil {
		return fmt.Errorf("migrations: %w", err)
	}
	slog.Info("migrations applied")

	// NATS
	queue, err := cfnats.Connect(ctx, cfg.NATS.URL)
	if err != nil {
		return fmt.Errorf("nats: %w", err)
	}
	defer func() { _ = queue.Close() }()

	// --- Agent Backends ---
	aider.Register(queue)

	// --- Services ---
	hub := ws.NewHub()
	store := postgres.NewStore(pool)
	projectSvc := service.NewProjectService(store)
	taskSvc := service.NewTaskService(store, queue)
	agentSvc := service.NewAgentService(store, queue, hub)

	// Start NATS subscribers (process results and streaming output from workers)
	cancelResults, err := agentSvc.StartResultSubscriber(ctx)
	if err != nil {
		return fmt.Errorf("result subscriber: %w", err)
	}
	defer cancelResults()

	cancelOutput, err := agentSvc.StartOutputSubscriber(ctx)
	if err != nil {
		return fmt.Errorf("output subscriber: %w", err)
	}
	defer cancelOutput()

	// --- HTTP ---
	llmClient := litellm.NewClient(cfg.LiteLLM.URL, cfg.LiteLLM.MasterKey)

	handlers := &cfhttp.Handlers{
		Projects: projectSvc,
		Tasks:    taskSvc,
		Agents:   agentSvc,
		LiteLLM:  llmClient,
	}

	r := chi.NewRouter()

	// Middleware
	r.Use(cfhttp.CORS(cfg.Server.CORSOrigin))
	r.Use(cfhttp.Logger)
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)
	r.Use(chimw.Timeout(30 * time.Second))

	// Health endpoint with service status
	r.Get("/health", healthHandler(cfg))

	// WebSocket endpoint
	r.Get("/ws", hub.HandleWS)

	// API routes
	cfhttp.MountRoutes(r, handlers)

	addr := ":" + cfg.Server.Port

	srv := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// Graceful shutdown
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	go func() {
		slog.Info("starting server", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server failed", "error", err)
		}
	}()

	<-done
	slog.Info("shutting down server")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return srv.Shutdown(shutdownCtx)
}

// healthHandler returns an http.HandlerFunc that reports service health.
func healthHandler(cfg *config.Config) http.HandlerFunc {
	type healthStatus struct {
		Status   string `json:"status"`
		Postgres string `json:"postgres"`
		NATS     string `json:"nats"`
		LiteLLM  string `json:"litellm"`
	}

	return func(w http.ResponseWriter, _ *http.Request) {
		status := healthStatus{
			Status:   "ok",
			Postgres: cfg.Postgres.DSN,
			NATS:     cfg.NATS.URL,
			LiteLLM:  cfg.LiteLLM.URL,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(status)
	}
}
