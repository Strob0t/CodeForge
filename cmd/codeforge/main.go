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
	"github.com/Strob0t/CodeForge/internal/service"
)

// config holds all runtime configuration loaded from environment variables.
type config struct {
	port             string
	corsOrigin       string
	dbDSN            string
	natsURL          string
	litellmURL       string
	litellmMasterKey string
}

func loadConfig() config {
	return config{
		port:             envOr("CODEFORGE_PORT", "8080"),
		corsOrigin:       envOr("CODEFORGE_CORS_ORIGIN", "http://localhost:3000"),
		dbDSN:            envOr("DATABASE_URL", "postgres://codeforge:codeforge_dev@localhost:5432/codeforge?sslmode=disable"),
		natsURL:          envOr("NATS_URL", "nats://localhost:4222"),
		litellmURL:       envOr("LITELLM_URL", "http://localhost:4000"),
		litellmMasterKey: envOr("LITELLM_MASTER_KEY", ""),
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := loadConfig()
	ctx := context.Background()

	// --- Infrastructure ---

	// PostgreSQL
	pool, err := postgres.NewPool(ctx, cfg.dbDSN)
	if err != nil {
		return fmt.Errorf("postgres: %w", err)
	}
	defer pool.Close()
	slog.Info("postgres connected")

	// Run migrations
	if err := postgres.RunMigrations(ctx, cfg.dbDSN); err != nil {
		return fmt.Errorf("migrations: %w", err)
	}
	slog.Info("migrations applied")

	// NATS
	queue, err := cfnats.Connect(ctx, cfg.natsURL)
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
	llmClient := litellm.NewClient(cfg.litellmURL, cfg.litellmMasterKey)

	handlers := &cfhttp.Handlers{
		Projects: projectSvc,
		Tasks:    taskSvc,
		Agents:   agentSvc,
		LiteLLM:  llmClient,
	}

	r := chi.NewRouter()

	// Middleware
	r.Use(cfhttp.CORS(cfg.corsOrigin))
	r.Use(cfhttp.Logger)
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)
	r.Use(chimw.Timeout(30 * time.Second))

	// Health endpoint with service status
	r.Get("/health", healthHandler(&cfg))

	// WebSocket endpoint
	r.Get("/ws", hub.HandleWS)

	// API routes
	cfhttp.MountRoutes(r, handlers)

	addr := ":" + cfg.port

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
func healthHandler(cfg *config) http.HandlerFunc {
	type healthStatus struct {
		Status   string `json:"status"`
		Postgres string `json:"postgres"`
		NATS     string `json:"nats"`
		LiteLLM  string `json:"litellm"`
	}

	return func(w http.ResponseWriter, _ *http.Request) {
		status := healthStatus{
			Status:   "ok",
			Postgres: cfg.dbDSN,
			NATS:     cfg.natsURL,
			LiteLLM:  cfg.litellmURL,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(status)
	}
}
