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

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Strob0t/CodeForge/internal/adapter/aider"
	cfhttp "github.com/Strob0t/CodeForge/internal/adapter/http"
	"github.com/Strob0t/CodeForge/internal/adapter/litellm"
	cfnats "github.com/Strob0t/CodeForge/internal/adapter/nats"
	"github.com/Strob0t/CodeForge/internal/adapter/postgres"
	"github.com/Strob0t/CodeForge/internal/adapter/ws"
	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/logger"
	"github.com/Strob0t/CodeForge/internal/middleware"
	"github.com/Strob0t/CodeForge/internal/resilience"
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
	pool, err := postgres.NewPool(ctx, cfg.Postgres)
	if err != nil {
		return fmt.Errorf("postgres: %w", err)
	}
	slog.Info("postgres connected",
		"max_conns", cfg.Postgres.MaxConns,
		"min_conns", cfg.Postgres.MinConns,
	)

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

	// --- Circuit Breakers ---
	natsBreaker := resilience.NewBreaker(cfg.Breaker.MaxFailures, cfg.Breaker.Timeout)
	llmBreaker := resilience.NewBreaker(cfg.Breaker.MaxFailures, cfg.Breaker.Timeout)
	queue.SetBreaker(natsBreaker)

	// --- Agent Backends ---
	aider.Register(queue)

	// --- Services ---
	hub := ws.NewHub()
	store := postgres.NewStore(pool)
	eventStore := postgres.NewEventStore(pool)
	projectSvc := service.NewProjectService(store)
	taskSvc := service.NewTaskService(store, queue)
	agentSvc := service.NewAgentService(store, queue, hub)
	agentSvc.SetEventStore(eventStore)

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
	llmClient.SetBreaker(llmBreaker)

	handlers := &cfhttp.Handlers{
		Projects: projectSvc,
		Tasks:    taskSvc,
		Agents:   agentSvc,
		LiteLLM:  llmClient,
	}

	r := chi.NewRouter()

	// Rate limiter
	rateLimiter := middleware.NewRateLimiter(cfg.Rate.RequestsPerSecond, cfg.Rate.Burst)

	// Middleware
	r.Use(cfhttp.CORS(cfg.Server.CORSOrigin))
	r.Use(middleware.RequestID)
	r.Use(cfhttp.Logger)
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)
	r.Use(chimw.Timeout(30 * time.Second))
	r.Use(rateLimiter.Handler)

	// Liveness (always 200)
	r.Get("/health", livenessHandler)

	// Readiness (pings DB, checks NATS, checks LiteLLM)
	r.Get("/health/ready", readinessHandler(pool, queue, llmClient))

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

// livenessHandler always returns 200 (Kubernetes liveness probe).
func livenessHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

// readinessHandler checks all dependencies and returns 503 if any are down.
func readinessHandler(pool *pgxpool.Pool, queue *cfnats.Queue, llm *litellm.Client) http.HandlerFunc {
	type serviceStatus struct {
		Status  string `json:"status"`
		Latency string `json:"latency,omitempty"`
	}

	type readiness struct {
		Status   string        `json:"status"`
		Postgres serviceStatus `json:"postgres"`
		NATS     serviceStatus `json:"nats"`
		LiteLLM  serviceStatus `json:"litellm"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		allOK := true
		resp := readiness{Status: "ready"}

		// PostgreSQL: ping
		pgStart := time.Now()
		if err := pool.Ping(r.Context()); err != nil {
			resp.Postgres = serviceStatus{Status: "down"}
			allOK = false
		} else {
			resp.Postgres = serviceStatus{
				Status:  "up",
				Latency: time.Since(pgStart).String(),
			}
		}

		// NATS: connection check
		if queue.IsConnected() {
			resp.NATS = serviceStatus{Status: "up"}
		} else {
			resp.NATS = serviceStatus{Status: "down"}
			allOK = false
		}

		// LiteLLM: health check
		llmStart := time.Now()
		healthy, _ := llm.Health(r.Context())
		if healthy {
			resp.LiteLLM = serviceStatus{
				Status:  "up",
				Latency: time.Since(llmStart).String(),
			}
		} else {
			resp.LiteLLM = serviceStatus{Status: "down"}
			allOK = false
		}

		httpStatus := http.StatusOK
		if !allOK {
			resp.Status = "not ready"
			httpStatus = http.StatusServiceUnavailable
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(httpStatus)
		_ = json.NewEncoder(w).Encode(resp)
	}
}
