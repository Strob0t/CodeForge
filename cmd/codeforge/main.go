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
	"github.com/Strob0t/CodeForge/internal/adapter/natskv"
	cfotel "github.com/Strob0t/CodeForge/internal/adapter/otel"
	"github.com/Strob0t/CodeForge/internal/adapter/postgres"
	ristrettoAdapter "github.com/Strob0t/CodeForge/internal/adapter/ristretto"
	"github.com/Strob0t/CodeForge/internal/adapter/tiered"
	"github.com/Strob0t/CodeForge/internal/adapter/ws"
	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/policy"
	"github.com/Strob0t/CodeForge/internal/git"
	"github.com/Strob0t/CodeForge/internal/logger"
	"github.com/Strob0t/CodeForge/internal/middleware"
	"github.com/Strob0t/CodeForge/internal/port/a2a"
	"github.com/Strob0t/CodeForge/internal/port/notifier"
	"github.com/Strob0t/CodeForge/internal/port/pmprovider"
	"github.com/Strob0t/CodeForge/internal/port/specprovider"
	"github.com/Strob0t/CodeForge/internal/resilience"
	"github.com/Strob0t/CodeForge/internal/secrets"
	"github.com/Strob0t/CodeForge/internal/service"
)

func main() {
	// Temporary bootstrap logger until config is loaded.
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	if err := run(); err != nil {
		// The async log handler is already closed (via defer in run()),
		// so we must write to stderr directly.
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	// Replace bootstrap logger with configured one.
	log, logCloser := logger.New(cfg.Logging)
	slog.SetDefault(log)
	defer logCloser.Close()

	slog.Info("config loaded",
		"port", cfg.Server.Port,
		"log_level", cfg.Logging.Level,
		"pg_max_conns", cfg.Postgres.MaxConns,
	)

	// --- OpenTelemetry ---
	otelShutdown, err := cfotel.InitTracer(cfotel.OTELConfig{
		Enabled:     cfg.OTEL.Enabled,
		Endpoint:    cfg.OTEL.Endpoint,
		ServiceName: cfg.OTEL.ServiceName,
		Insecure:    cfg.OTEL.Insecure,
		SampleRate:  cfg.OTEL.SampleRate,
	})
	if err != nil {
		return fmt.Errorf("otel: %w", err)
	}
	defer func() {
		if err := otelShutdown(context.Background()); err != nil {
			slog.Error("otel shutdown error", "error", err)
		}
	}()

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

	// Idempotency KV store
	idempotencyKV, err := queue.KeyValue(ctx, cfg.Idempotency.Bucket, cfg.Idempotency.TTL)
	if err != nil {
		return fmt.Errorf("idempotency kv: %w", err)
	}

	// --- Cache Layer ---
	l1Cache, err := ristrettoAdapter.New(cfg.Cache.L1MaxSizeMB * 1024 * 1024)
	if err != nil {
		return fmt.Errorf("ristretto cache: %w", err)
	}
	defer l1Cache.Close()
	cacheKV, err := queue.KeyValue(ctx, cfg.Cache.L2Bucket, cfg.Cache.L2TTL)
	if err != nil {
		return fmt.Errorf("cache kv: %w", err)
	}
	l2Cache := natskv.New(cacheKV)
	_ = tiered.New(l1Cache, l2Cache, 5*time.Minute) // appCache available for future service injection
	slog.Info("cache layer initialized", "l1_max_mb", cfg.Cache.L1MaxSizeMB, "l2_bucket", cfg.Cache.L2Bucket)

	// --- Circuit Breakers ---
	natsBreaker := resilience.NewBreaker(cfg.Breaker.MaxFailures, cfg.Breaker.Timeout)
	llmBreaker := resilience.NewBreaker(cfg.Breaker.MaxFailures, cfg.Breaker.Timeout)
	queue.SetBreaker(natsBreaker)

	// --- Git Worker Pool ---
	gitPool := git.NewPool(cfg.Git.MaxConcurrent)
	slog.Info("git worker pool initialized", "max_concurrent", cfg.Git.MaxConcurrent)

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

	// --- Policy Service ---
	var customPolicies []policy.PolicyProfile
	if cfg.Policy.CustomDir != "" {
		loaded, err := policy.LoadFromDirectory(cfg.Policy.CustomDir)
		if err != nil {
			return fmt.Errorf("policy custom dir: %w", err)
		}
		customPolicies = loaded
	}
	policySvc := service.NewPolicyService(cfg.Policy.DefaultProfile, customPolicies)
	slog.Info("policy service initialized",
		"default_profile", cfg.Policy.DefaultProfile,
		"profiles", len(policySvc.ListProfiles()),
	)

	// --- Runtime Service (Phase 4B + 4C) ---
	runtimeSvc := service.NewRuntimeService(store, queue, hub, eventStore, policySvc, &cfg.Runtime)
	deliverSvc := service.NewDeliverService(store, &cfg.Runtime, gitPool)
	runtimeSvc.SetDeliverService(deliverSvc)

	// Checkpoint Service (Phase 4A/4C)
	checkpointSvc := service.NewCheckpointService(gitPool)
	runtimeSvc.SetCheckpointService(checkpointSvc)

	// Sandbox Service (Phase 4B)
	sandboxSvc := service.NewSandboxService(service.SandboxConfig{
		MemoryMB:    cfg.Runtime.Sandbox.MemoryMB,
		CPUQuota:    cfg.Runtime.Sandbox.CPUQuota,
		PidsLimit:   cfg.Runtime.Sandbox.PidsLimit,
		StorageGB:   cfg.Runtime.Sandbox.StorageGB,
		NetworkMode: cfg.Runtime.Sandbox.NetworkMode,
		Image:       cfg.Runtime.Sandbox.Image,
	})
	runtimeSvc.SetSandboxService(sandboxSvc)

	runtimeCancels, err := runtimeSvc.StartSubscribers(ctx)
	if err != nil {
		return fmt.Errorf("runtime subscribers: %w", err)
	}
	slog.Info("runtime service initialized", "subscribers", len(runtimeCancels))

	// --- Orchestrator Service (Phase 5A) ---
	orchSvc := service.NewOrchestratorService(store, hub, eventStore, runtimeSvc, &cfg.Orchestrator)
	runtimeSvc.SetOnRunComplete(orchSvc.HandleRunCompleted)
	slog.Info("orchestrator service initialized",
		"max_parallel", cfg.Orchestrator.MaxParallel,
		"ping_pong_max_rounds", cfg.Orchestrator.PingPongMaxRounds,
	)

	// Start NATS subscribers (process results and streaming output from workers)
	cancelResults, err := agentSvc.StartResultSubscriber(ctx)
	if err != nil {
		return fmt.Errorf("result subscriber: %w", err)
	}

	cancelOutput, err := agentSvc.StartOutputSubscriber(ctx)
	if err != nil {
		return fmt.Errorf("output subscriber: %w", err)
	}

	// --- Secrets Vault ---
	vault, err := secrets.NewVault(secrets.EnvLoader("LITELLM_MASTER_KEY"))
	if err != nil {
		return fmt.Errorf("secrets vault: %w", err)
	}
	slog.Info("secrets vault initialized")

	// --- HTTP ---
	llmClient := litellm.NewClient(cfg.LiteLLM.URL, cfg.LiteLLM.MasterKey)
	llmClient.SetBreaker(llmBreaker)
	llmClient.SetVault(vault)

	// --- Meta-Agent Service (Phase 5B) ---
	metaAgentSvc := service.NewMetaAgentService(store, llmClient, orchSvc, &cfg.Orchestrator)
	slog.Info("meta-agent service initialized",
		"mode", cfg.Orchestrator.Mode,
		"decompose_model", cfg.Orchestrator.DecomposeModel,
	)

	// --- Pool Manager + Task Planner (Phase 5C) ---
	poolManagerSvc := service.NewPoolManagerService(store, hub, &cfg.Orchestrator)
	taskPlannerSvc := service.NewTaskPlannerService(metaAgentSvc, poolManagerSvc, store, &cfg.Orchestrator)
	slog.Info("pool manager and task planner initialized",
		"max_team_size", cfg.Orchestrator.MaxTeamSize,
	)

	// --- Context Optimizer + Shared Context (Phase 5D) ---
	contextOptSvc := service.NewContextOptimizerService(store, &cfg.Orchestrator)
	sharedCtxSvc := service.NewSharedContextService(store, hub, queue)
	runtimeSvc.SetContextOptimizer(contextOptSvc)
	slog.Info("context optimizer and shared context initialized",
		"default_budget", cfg.Orchestrator.DefaultContextBudget,
		"prompt_reserve", cfg.Orchestrator.PromptReserve,
	)

	// --- RepoMap Service (Phase 6A) ---
	repoMapSvc := service.NewRepoMapService(store, queue, hub, &cfg.Orchestrator)
	repoMapCancel, err := repoMapSvc.StartSubscriber(ctx)
	if err != nil {
		return fmt.Errorf("repomap subscriber: %w", err)
	}
	slog.Info("repomap service initialized", "token_budget", cfg.Orchestrator.RepoMapTokenBudget)

	// --- Retrieval Service (Phase 6B) ---
	retrievalSvc := service.NewRetrievalService(store, queue, hub, &cfg.Orchestrator)
	retrievalCancels, err := retrievalSvc.StartSubscribers(ctx)
	if err != nil {
		return fmt.Errorf("retrieval subscribers: %w", err)
	}
	contextOptSvc.SetRetrieval(retrievalSvc)
	slog.Info("retrieval service initialized")

	// --- Graph Service (Phase 6D) ---
	graphSvc := service.NewGraphService(store, queue, hub, &cfg.Orchestrator)
	graphCancels, err := graphSvc.StartSubscribers(ctx)
	if err != nil {
		return fmt.Errorf("graph subscribers: %w", err)
	}
	contextOptSvc.SetGraph(graphSvc)
	slog.Info("graph service initialized", "enabled", cfg.Orchestrator.GraphEnabled)

	// --- Wire SharedContext into PoolManager + Orchestrator (Phase 5E) ---
	poolManagerSvc.SetSharedContext(sharedCtxSvc)
	orchSvc.SetSharedContext(sharedCtxSvc)

	// --- Mode Service (Phase 5E) ---
	modeSvc := service.NewModeService()
	slog.Info("mode service initialized", "modes", len(modeSvc.List()))

	// --- Spec & PM Providers (Phase 9A) ---
	var specProvs []specprovider.Provider
	for _, name := range specprovider.Available() {
		p, err := specprovider.New(name, nil)
		if err != nil {
			slog.Warn("failed to create spec provider", "name", name, "error", err)
			continue
		}
		specProvs = append(specProvs, p)
	}
	var pmProvs []pmprovider.Provider
	for _, name := range pmprovider.Available() {
		p, err := pmprovider.New(name, nil)
		if err != nil {
			slog.Warn("failed to create PM provider", "name", name, "error", err)
			continue
		}
		pmProvs = append(pmProvs, p)
	}
	slog.Info("spec/pm providers initialized",
		"spec_providers", len(specProvs),
		"pm_providers", len(pmProvs),
	)

	// --- Roadmap Service (Phase 8) ---
	roadmapSvc := service.NewRoadmapService(store, hub, specProvs, pmProvs)
	slog.Info("roadmap service initialized")

	// --- Tenant Service ---
	tenantSvc := service.NewTenantService(store)
	slog.Info("tenant service initialized")

	// --- Branch Protection Service ---
	branchProtSvc := service.NewBranchProtectionService(store)
	slog.Info("branch protection service initialized")

	// --- Replay & Session Services ---
	replaySvc := service.NewReplayService(store, eventStore)
	sessionSvc := service.NewSessionService(store, eventStore)
	slog.Info("replay and session services initialized")

	// --- VCS Webhook & Sync Services ---
	vcsWebhookSvc := service.NewVCSWebhookService(hub)
	syncSvc := service.NewSyncService(store)
	pmWebhookSvc := service.NewPMWebhookService(hub, syncSvc)
	slog.Info("vcs webhook, pm webhook, and sync services initialized")

	// --- Notification Service ---
	var notifiers []notifier.Notifier
	for _, name := range notifier.Available() {
		cfgMap := map[string]string{}
		switch name {
		case "slack":
			cfgMap["webhook_url"] = cfg.Notification.SlackWebhookURL
		case "discord":
			cfgMap["webhook_url"] = cfg.Notification.DiscordWebhookURL
		}
		n, err := notifier.New(name, cfgMap)
		if err != nil {
			slog.Warn("failed to create notifier", "name", name, "error", err)
			continue
		}
		notifiers = append(notifiers, n)
	}
	notificationSvc := service.NewNotificationService(notifiers, cfg.Notification.EnabledEvents)
	slog.Info("notification service initialized", "notifiers", notificationSvc.NotifierCount())

	// --- Cost Service (Phase 7) ---
	costSvc := service.NewCostService(store)

	handlers := &cfhttp.Handlers{
		Projects:         projectSvc,
		Tasks:            taskSvc,
		Agents:           agentSvc,
		LiteLLM:          llmClient,
		Policies:         policySvc,
		Runtime:          runtimeSvc,
		Orchestrator:     orchSvc,
		MetaAgent:        metaAgentSvc,
		PoolManager:      poolManagerSvc,
		TaskPlanner:      taskPlannerSvc,
		ContextOptimizer: contextOptSvc,
		SharedContext:    sharedCtxSvc,
		Modes:            modeSvc,
		RepoMap:          repoMapSvc,
		Retrieval:        retrievalSvc,
		Graph:            graphSvc,
		Events:           eventStore,
		Cost:             costSvc,
		Roadmap:          roadmapSvc,
		Tenants:          tenantSvc,
		BranchProtection: branchProtSvc,
		Replay:           replaySvc,
		Sessions:         sessionSvc,
		VCSWebhook:       vcsWebhookSvc,
		Sync:             syncSvc,
		PMWebhook:        pmWebhookSvc,
		Notification:     notificationSvc,
	}

	r := chi.NewRouter()

	// Rate limiter
	rateLimiter := middleware.NewRateLimiter(cfg.Rate.RequestsPerSecond, cfg.Rate.Burst)
	rateLimiterCleanup := rateLimiter.StartCleanup(cfg.Rate.CleanupInterval, cfg.Rate.MaxIdleTime)
	defer rateLimiterCleanup()

	// Middleware
	r.Use(cfhttp.CORS(cfg.Server.CORSOrigin))
	if cfg.OTEL.Enabled {
		r.Use(cfotel.HTTPMiddleware(cfg.OTEL.ServiceName))
	}
	r.Use(middleware.RequestID)
	r.Use(middleware.TenantID)
	r.Use(cfhttp.Logger)
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)
	r.Use(chimw.Timeout(30 * time.Second))
	r.Use(rateLimiter.Handler)
	r.Use(middleware.Idempotency(idempotencyKV))

	// Liveness (always 200)
	r.Get("/health", livenessHandler)

	// Readiness (pings DB, checks NATS, checks LiteLLM)
	r.Get("/health/ready", readinessHandler(pool, queue, llmClient))

	// WebSocket endpoint
	r.Get("/ws", hub.HandleWS)

	// API routes
	cfhttp.MountRoutes(r, handlers)

	// A2A protocol routes (root level, not under /api/v1)
	if cfg.A2A.Enabled {
		a2aHandler := a2a.NewHandler("http://localhost:" + cfg.Server.Port)
		a2aHandler.MountRoutes(r)
		slog.Info("a2a protocol enabled")
	}

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

	// ConfigHolder for hot reload support.
	cfgHolder := config.NewHolder(cfg, config.DefaultConfigFile)

	// SIGHUP triggers hot reload of config and secrets vault
	sighup := make(chan os.Signal, 1)
	signal.Notify(sighup, syscall.SIGHUP)
	go func() {
		for range sighup {
			slog.Info("SIGHUP received, reloading config and secrets")
			if err := cfgHolder.Reload(); err != nil {
				slog.Error("config reload failed", "error", err)
			} else {
				slog.Info("config reloaded successfully")
			}
			if err := vault.Reload(); err != nil {
				slog.Error("secrets reload failed", "error", err)
			} else {
				slog.Info("secrets reloaded successfully")
			}
		}
	}()

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
	for _, cancel := range runtimeCancels {
		cancel()
	}
	cancelResults()
	cancelOutput()
	repoMapCancel()
	for _, cancel := range retrievalCancels {
		cancel()
	}
	for _, cancel := range graphCancels {
		cancel()
	}

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
