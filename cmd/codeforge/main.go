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
	"github.com/Strob0t/CodeForge/internal/logger"
	"github.com/Strob0t/CodeForge/internal/middleware"
	"github.com/Strob0t/CodeForge/internal/service"
)

func main() {
	// Temporary bootstrap logger until config is loaded.
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

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

	// Replace bootstrap logger with configured one.
	slog.SetDefault(logger.New(cfg.Logging))

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

	cancelOutput, err := agentSvc.StartOutputSubscriber(ctx)
	if err != nil {
		return fmt.Errorf("output subscriber: %w", err)
	}

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
	r.Use(middleware.RequestID)
	r.Use(cfhttp.Logger)
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

	// Wait for interrupt signal
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	go func() {
		slog.Info("starting server", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server failed", "error", err)
		}
	}()

	<-done

	// --- Ordered Graceful Shutdown ---
	// Phase 1: Stop accepting new HTTP requests
	slog.Info("shutdown phase 1: stopping HTTP server")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("http shutdown error", "error", err)
	}

	// Phase 2: Cancel NATS subscribers (stop processing new messages)
	slog.Info("shutdown phase 2: cancelling NATS subscribers")
	cancelResults()
	cancelOutput()

	// Phase 3: Drain NATS (flush pending publishes, wait for acks)
	slog.Info("shutdown phase 3: draining NATS connection")
	if err := queue.Drain(); err != nil {
		slog.Error("nats drain error", "error", err)
	}

	// Phase 4: Close database (last, so in-flight queries can complete)
	slog.Info("shutdown phase 4: closing database pool")
	pool.Close()

	slog.Info("shutdown complete")
	return nil
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
