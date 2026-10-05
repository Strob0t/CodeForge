package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"gopkg.in/yaml.v3"

	"github.com/a2aproject/a2a-go/a2asrv"
	"github.com/jackc/pgx/v5/pgxpool"

	cfa2a "github.com/Strob0t/CodeForge/internal/adapter/a2a"
	"github.com/Strob0t/CodeForge/internal/adapter/aider"
	cfauth "github.com/Strob0t/CodeForge/internal/adapter/auth"
	"github.com/Strob0t/CodeForge/internal/adapter/copilot"
	"github.com/Strob0t/CodeForge/internal/adapter/goose"
	cfhttp "github.com/Strob0t/CodeForge/internal/adapter/http"
	"github.com/Strob0t/CodeForge/internal/adapter/litellm"
	lspAdapter "github.com/Strob0t/CodeForge/internal/adapter/lsp"
	cfmcp "github.com/Strob0t/CodeForge/internal/adapter/mcp"
	cfnats "github.com/Strob0t/CodeForge/internal/adapter/nats"
	"github.com/Strob0t/CodeForge/internal/adapter/opencode"
	"github.com/Strob0t/CodeForge/internal/adapter/openhands"
	cfotel "github.com/Strob0t/CodeForge/internal/adapter/otel"
	"github.com/Strob0t/CodeForge/internal/adapter/plandex"
	"github.com/Strob0t/CodeForge/internal/adapter/postgres"
	"github.com/Strob0t/CodeForge/internal/adapter/ws"
	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/crypto"
	lspDomain "github.com/Strob0t/CodeForge/internal/domain/lsp"
	"github.com/Strob0t/CodeForge/internal/domain/microagent"
	"github.com/Strob0t/CodeForge/internal/domain/mode"
	"github.com/Strob0t/CodeForge/internal/domain/pipeline"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/prompt"
	"github.com/Strob0t/CodeForge/internal/git"
	"github.com/Strob0t/CodeForge/internal/logger"
	"github.com/Strob0t/CodeForge/internal/middleware"
	llmPort "github.com/Strob0t/CodeForge/internal/port/llm"
	lspPort "github.com/Strob0t/CodeForge/internal/port/lsp"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/port/notifier"
	"github.com/Strob0t/CodeForge/internal/port/pmprovider"
	"github.com/Strob0t/CodeForge/internal/port/specprovider"
	"github.com/Strob0t/CodeForge/internal/proctemp"
	"github.com/Strob0t/CodeForge/internal/resilience"
	"github.com/Strob0t/CodeForge/internal/secrets"
	"github.com/Strob0t/CodeForge/internal/service"
	cfversion "github.com/Strob0t/CodeForge/internal/version"
)

func main() {
	// Temporary bootstrap logger until config is loaded.
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	// Dispatch admin subcommands before starting the server.
	if len(os.Args) > 1 && os.Args[1] == "admin" {
		if err := runAdmin(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "admin: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if err := run(); err != nil {
		// The async log handler is already closed (via defer in run()),
		// so we must write to stderr directly.
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	setWorkspaceUmask()

	flags, err := config.ParseFlags(os.Args[1:])
	if err != nil {
		return fmt.Errorf("flags: %w", err)
	}

	cfg, _, err := config.LoadWithCLI(flags)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	// Replace bootstrap logger with configured one.
	log, logCloser, logDropped := logger.New(cfg.Logging)
	slog.SetDefault(log)
	defer logCloser.Close()

	slog.Info("config loaded",
		"port", cfg.Server.Port,
		"log_level", cfg.Logging.Level,
		"pg_max_conns", cfg.Postgres.MaxConns,
	)

	if cfg.AppEnv != "" && cfg.AppEnv != "development" {
		if strings.Contains(cfg.Postgres.DSN, "codeforge_dev") {
			slog.Warn("production detected with default database password - change POSTGRES_PASSWORD")
		}
	}

	if cfg.InternalKey == "" {
		slog.Warn("CODEFORGE_INTERNAL_KEY not set — Python worker API calls will fail with 401")
	}

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

	metrics, err := cfotel.NewMetrics()
	if err != nil {
		return fmt.Errorf("otel metrics: %w", err)
	}

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
	queue, err := cfnats.Connect(ctx, cfg.NATS.URL, cfg.NATS.StreamMaxBytes)
	if err != nil {
		return fmt.Errorf("nats: %w", err)
	}
	// The tool-call handler waits up to the HITL approval timeout; its message
	// must stay in progress that long (ADR-016).
	queue.SetMaxHandlerDuration(cfg.Runtime.ApprovalTimeout())

	// Idempotency KV store
	idempotencyKV, err := queue.KeyValue(ctx, cfg.Idempotency.Bucket, cfg.Idempotency.TTL)
	if err != nil {
		return fmt.Errorf("idempotency kv: %w", err)
	}

	// --- Circuit Breakers ---
	natsBreaker := resilience.NewBreaker(cfg.Breaker.MaxFailures, cfg.Breaker.Timeout)
	llmBreaker := resilience.NewBreaker(cfg.Breaker.MaxFailures, cfg.Breaker.Timeout)
	queue.SetBreaker(natsBreaker)

	// --- Git Worker Pool ---
	gitPool := git.NewPool(cfg.Git.MaxConcurrent)
	slog.Info("git worker pool initialized", "max_concurrent", cfg.Git.MaxConcurrent)

	// --- Agent Backends ---
	aider.Register(queue)
	goose.Register(queue)
	opencode.Register(queue)
	openhands.Register(queue)
	plandex.Register(queue)

	// --- Services ---
	// WebSocket upgrades authenticate with single-use tickets from
	// POST /api/v1/ws/ticket; each connection is bound to the ticket's tenant.
	wsTickets := ws.NewTicketStore(ws.DefaultTicketTTL)
	wsTickets.StartCleanup(ctx)
	hub := ws.NewHub(cfg.Server.CORSOrigin, wsTickets)
	store := postgres.NewStore(pool)
	eventStore := postgres.NewEventStore(pool)
	toolUIDSvc := service.NewToolUIDService(store, toolACLsRequired(cfg))
	projectSvc := service.NewProjectService(store, cfg.Workspace.Root)
	projectSvc.SetAdoptRoots(cfg.Workspace.AdoptRoots)
	projectSvc.SetToolUIDs(toolUIDSvc)
	if err := toolUIDSvc.PrepareAtStartup(ctx, projectSvc.WorkspaceRoot(), store); err != nil {
		return fmt.Errorf("per-tenant tool identities: %w", err)
	}
	// With tool ACLs required a deleted project's workspace is removed by the
	// worker as the tenant's tool UID (KI-96 D11).
	workspaceDeletionSvc := service.NewWorkspaceDeletionService(store, queue, toolUIDSvc)
	projectSvc.SetWorkspaceDeletions(workspaceDeletionSvc)
	taskSvc := service.NewTaskService(store, queue)
	agentSvc := service.NewAgentService(store, queue, hub)
	agentSvc.SetToolUIDs(toolUIDSvc)
	agentSvc.SetEventStore(eventStore)
	agentSvc.SetRuntimeConfig(&cfg.Runtime)

	// --- Policy Service ---
	policySvc := service.NewPolicyService(cfg.Policy.DefaultProfile, nil)
	// Profiles created via the API and Allow-Always rules are written back to
	// the directory they are loaded from, so they survive a restart.
	if err := policySvc.LoadPolicyDir(cfg.Policy.CustomDir); err != nil {
		return fmt.Errorf("policy custom dir: %w", err)
	}
	slog.Info("policy service initialized",
		"default_profile", cfg.Policy.DefaultProfile,
		"profiles", len(policySvc.ListProfiles(context.Background())),
		"policy_dir", cfg.Policy.CustomDir,
	)
	if cfg.Policy.CustomDir == "" {
		slog.Warn("policy.custom_dir is not set: custom policy profiles stay in memory and allow-always is disabled")
	}

	// --- Active Work Service (Phase 24) ---
	activeWorkSvc := service.NewActiveWorkService(store, hub)
	slog.Info("active work service initialized")

	// --- Routing Service (Phase 26) ---
	routingSvc := service.NewRoutingService(store)
	slog.Info("routing service initialized")

	// --- Quarantine Service (Phase 23B) ---
	quarantineSvc := service.NewQuarantineService(store, queue, hub, cfg.Quarantine)
	slog.Info("quarantine service initialized", "enabled", cfg.Quarantine.Enabled)

	// --- Runtime Service (Phase 4B + 4C) ---
	runtimeSvc := service.NewRuntimeService(store, queue, hub, eventStore, policySvc, &cfg.Runtime)
	runtimeSvc.SetQuarantineService(quarantineSvc)
	runtimeSvc.SetToolUIDs(toolUIDSvc)
	runtimeSvc.SetMetrics(metrics)
	deliverSvc := service.NewDeliverService(store, &cfg.Runtime, gitPool)
	runtimeSvc.SetDeliverService(deliverSvc)

	// Private temporary files (checkpoint indexes, svn config) live in one
	// directory per process; those of earlier processes are removed.
	if removed, err := proctemp.RemoveStale(os.TempDir()); err != nil {
		slog.Warn("stale temp directories not removed", "error", err)
	} else if removed > 0 {
		slog.Info("stale temp directories removed", "count", removed)
	}

	// --- Handoffs (Phase 23B, KI-15) ---
	// The Go Core handles the workers' handoff_to calls (handoff.request):
	// trust and quarantine, the target agent's task and run, its inbox and
	// the handoff.status events of the War Room.
	handoffSvc := service.NewHandoffService(store, queue, hub)
	handoffSvc.SetQuarantineService(quarantineSvc)
	handoffSvc.SetRunStarter(runtimeSvc)

	// Checkpoint Service (Phase 4A/4C)
	checkpointSvc := service.NewCheckpointService(gitPool)
	runtimeSvc.SetCheckpointService(checkpointSvc)
	// The quality gate watchdog tells queued gates from lost ones by the
	// backlog of the gate subjects.
	runtimeSvc.SetBacklogProbe(queue)

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

	cancelAgentOutput, err := agentSvc.StartAgentOutputSubscriber(ctx)
	if err != nil {
		return fmt.Errorf("agent output subscriber: %w", err)
	}

	cancelTaskHeartbeats, err := agentSvc.StartHeartbeatSubscriber(ctx)
	if err != nil {
		return fmt.Errorf("task heartbeat subscriber: %w", err)
	}

	cancelTaskDeadLetters, err := agentSvc.StartDeadLetterSubscriber(ctx)
	if err != nil {
		return fmt.Errorf("task dead-letter subscriber: %w", err)
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
	providerKeys, err := llmPort.NewProviderKeys(cfg.LiteLLM.KeyedProviders, os.Getenv)
	if err != nil {
		return err
	}
	llmClient.SetProviderKeys(providerKeys)
	slog.Info("llm providers with an API key", "providers", providerKeys.Names())

	// --- Review Router Service (Phase 21A) ---
	reviewRouterSvc := service.NewReviewRouterService(llmClient, &cfg.Orchestrator, &cfg.Limits)
	orchSvc.SetReviewRouter(reviewRouterSvc)

	// --- Meta-Agent Service (Phase 5B) ---
	metaAgentSvc := service.NewMetaAgentService(store, llmClient, orchSvc, &cfg.Orchestrator, &cfg.Limits)
	slog.Info("meta-agent service initialized",
		"mode", cfg.Orchestrator.Mode,
		"decompose_model", cfg.Orchestrator.DecomposeModel,
	)

	// --- Pool Manager + Task Planner (Phase 5C) ---
	poolManagerSvc := service.NewPoolManagerService(store, hub, &cfg.Orchestrator)
	taskPlannerSvc := service.NewTaskPlannerService(metaAgentSvc, poolManagerSvc, store, &cfg.Orchestrator, &cfg.Limits)
	slog.Info("pool manager and task planner initialized",
		"max_team_size", cfg.Orchestrator.MaxTeamSize,
	)

	// --- Context Optimizer + Shared Context (Phase 5D) ---
	contextOptSvc := service.NewContextOptimizerService(store, &cfg.Orchestrator, &cfg.Limits)
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
	retrievalSvc := service.NewRetrievalService(store, queue, hub, &cfg.Orchestrator, &cfg.Limits)
	retrievalSvc.SetEventStore(eventStore)
	retrievalCancels, err := retrievalSvc.StartSubscribers(ctx)
	if err != nil {
		return fmt.Errorf("retrieval subscribers: %w", err)
	}
	contextOptSvc.SetRetrieval(retrievalSvc)
	slog.Info("retrieval service initialized")

	// --- Graph Service (Phase 6D) ---
	graphSvc := service.NewGraphService(store, queue, hub, &cfg.Orchestrator, &cfg.Limits)
	graphCancels, err := graphSvc.StartSubscribers(ctx)
	if err != nil {
		return fmt.Errorf("graph subscribers: %w", err)
	}
	contextOptSvc.SetGraph(graphSvc)
	slog.Info("graph service initialized", "enabled", cfg.Orchestrator.GraphEnabled)

	// --- Knowledge Base Service (Phase 12K) ---
	kbSvc := service.NewKnowledgeBaseService(store, cfg.Knowledge.ContentRoot)
	kbSvc.SetRetrieval(retrievalSvc)
	contextOptSvc.SetKnowledgeBases(kbSvc)
	retrievalSvc.SetKBUpdater(store)
	slog.Info("knowledge base service initialized")

	// --- Scope Service (Phase 12D) ---
	scopeSvc := service.NewScopeService(store)
	scopeSvc.SetRetrieval(retrievalSvc)
	scopeSvc.SetGraph(graphSvc)
	scopeSvc.SetKnowledgeBase(kbSvc)
	slog.Info("scope service initialized")

	// --- LSP Service ---
	var lspSvc *service.LSPService
	if cfg.LSP.Enabled {
		lspCfg := &cfg.LSP
		lspFactory := func(language string, serverCfg lspDomain.LanguageServerConfig, workspacePath string) lspPort.Client {
			return lspAdapter.NewClient(language, serverCfg, lspCfg, workspacePath)
		}
		lspSvc = service.NewLSPService(lspCfg, hub, store, lspFactory)
		contextOptSvc.SetLSP(lspSvc)
		slog.Info("lsp service initialized")
	}

	// --- Wire SharedContext into PoolManager + Orchestrator (Phase 5E) ---
	poolManagerSvc.SetSharedContext(sharedCtxSvc)
	orchSvc.SetSharedContext(sharedCtxSvc)
	// A team ends with its plan (KI-33).
	orchSvc.AddOnPlanComplete(poolManagerSvc.PlanEnded)

	// --- Mode Service (Phase 5E) ---
	modeSvc := service.NewModeService()
	runtimeSvc.SetModeService(modeSvc)
	runtimeSvc.SetToolOutputMaxChars(cfg.Agent.ToolOutputMaxChars)
	handoffSvc.SetModeService(modeSvc)
	// Auto-load custom modes from .codeforge/modes/ directory.
	if customModeFiles, globErr := filepath.Glob(".codeforge/modes/*.yaml"); globErr == nil {
		for _, f := range customModeFiles {
			data, readErr := os.ReadFile(filepath.Clean(f))
			if readErr != nil {
				slog.Warn("failed to read custom mode file", "file", f, "error", readErr)
				continue
			}
			var m mode.Mode
			if yamlErr := yaml.Unmarshal(data, &m); yamlErr != nil {
				slog.Warn("failed to parse custom mode file", "file", f, "error", yamlErr)
				continue
			}
			if err := modeSvc.Register(&m); err != nil {
				slog.Debug("skip custom mode (conflict or invalid)", "id", m.ID, "file", f, "error", err)
				continue
			}
			slog.Info("custom mode loaded", "id", m.ID, "file", f)
		}
	}
	slog.Info("mode service initialized", "modes", len(modeSvc.List()))

	// --- Pipeline Service (Phase 12F) ---
	pipelineSvc := service.NewPipelineService(modeSvc)
	if cfg.Workspace.PipelineDir != "" {
		templates, err := pipeline.LoadFromDirectory(cfg.Workspace.PipelineDir)
		if err != nil {
			slog.Warn("failed to load custom pipeline templates", "dir", cfg.Workspace.PipelineDir, "error", err)
		}
		for i := range templates {
			if err := pipelineSvc.Register(&templates[i]); err != nil {
				slog.Warn("failed to register pipeline template", "id", templates[i].ID, "error", err)
			}
		}
	}
	slog.Info("pipeline service initialized", "templates", len(pipelineSvc.List()))

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
	if err := setPMOutboundPolicy(cfg.PM.AllowedPrivateHosts); err != nil {
		return fmt.Errorf("pm.allowed_private_hosts: %w", err)
	}
	var pmProvs []pmprovider.Provider
	pmConfigs := map[string]map[string]string{
		"plane": {"api_token": cfg.Plane.APIToken, "base_url": cfg.Plane.BaseURL},
	}
	for _, name := range pmprovider.Available() {
		p, err := pmprovider.New(name, pmConfigs[name])
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
	projectSvc.SetSpecDetector(service.NewRoadmapSpecDetector(roadmapSvc))
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
	pmWebhookSvc := service.NewPMWebhookService(hub, syncSvc, pmConfigs)
	slog.Info("vcs webhook, pm webhook, and sync services initialized")

	// --- Review Service (Phase 12I) ---
	reviewSvc := service.NewReviewService(store, pipelineSvc, orchSvc, hub, eventStore)
	vcsWebhookSvc.SetReviewService(reviewSvc)
	orchSvc.SetOnPlanComplete(reviewSvc.HandlePlanComplete)
	reviewSvc.StartCron(ctx)
	defer reviewSvc.StopCron()
	slog.Info("review service initialized")

	// --- Boundary & Review Pipeline Services (Phase 31, KI-17) ---
	boundarySvc := service.NewBoundaryService(store)
	reviewPipelineSvc := service.NewReviewPipelineService(store, pipelineSvc, orchSvc, poolManagerSvc, gitPool, hub,
		service.DefaultDiffImpactConfig())
	orchSvc.SetStepGate(reviewPipelineSvc.GateStep)
	orchSvc.SetStepPreparer(reviewPipelineSvc)
	reviewPipelineSvc.SetRunEnds(runtimeSvc)
	runtimeSvc.SetOnWorkerStopped(reviewPipelineSvc.WorkerStopped)
	orchSvc.AddOnPlanComplete(reviewPipelineSvc.PlanEnded)
	reviewTriggerSvc := service.NewReviewTriggerService(store, reviewPipelineSvc)
	slog.Info("boundary and review pipeline services initialized")

	// Wire auto-index dependencies into ProjectService so it can
	// trigger background indexing without going through the HTTP layer.
	projectSvc.SetRepoMapIndexer(repoMapSvc)
	projectSvc.SetRetrievalIndexer(retrievalSvc)
	projectSvc.SetGraphBuilder(graphSvc)
	projectSvc.SetReviewTriggerer(reviewTriggerSvc)

	// --- GEMMAS Evaluation Hook (Phase 20G) ---
	evalSvc := service.NewEvaluationService(store, eventStore, queue)
	orchSvc.AddOnPlanComplete(evalSvc.HandlePlanComplete)
	slog.Info("evaluation service initialized")

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
	dashboardSvc := service.NewDashboardService(store)

	// --- Settings Service ---
	settingsSvc := service.NewSettingsService(store)

	// --- VCS Account Service ---
	vcsKey, err := crypto.DeriveKey(cfg.Auth.JWTSecret, nil, "codeforge/vcsaccount/v1")
	if err != nil {
		return fmt.Errorf("derive vcs encryption key: %w", err)
	}
	vcsAccountSvc := service.NewVCSAccountService(store, vcsKey)

	// --- Inbound webhooks (KI-85): registered per project, their secrets
	// and PM API tokens encrypted like VCS account tokens (own key). ---
	webhookKey, err := crypto.DeriveKey(cfg.Auth.JWTSecret, nil, "codeforge/webhook/v1")
	if err != nil {
		return fmt.Errorf("derive webhook encryption key: %w", err)
	}
	webhookSvc := service.NewWebhookService(store, webhookKey, vcsWebhookSvc, pmWebhookSvc, cfg.Webhook.DeliveryRetention)
	if removed := cfg.Webhook.RemovedGlobalSecrets(); len(removed) > 0 {
		slog.Warn("the global webhook routes were removed (KI-85) and these settings are ignored - register a webhook per project "+
			"(POST /api/v1/projects/{id}/webhooks) and point the provider at its URL with its secret",
			"ignored", removed)
	}

	// --- GitHub OAuth web flow (KI-55): connects a GitHub account as a VCS
	// account; its token is encrypted with the VCS account key. ---
	var githubOAuthSvc *service.GitHubOAuthService
	if cfg.GitHub.WebFlowConfigured() {
		githubOAuthSvc = service.NewGitHubOAuthService(service.GitHubOAuthConfig{
			ClientID:     cfg.GitHub.ClientID,
			ClientSecret: cfg.GitHub.ClientSecret,
			RedirectURI:  cfg.GitHub.CallbackURL,
			Scopes:       []string{"repo", "read:user"},
		}, store, vcsKey)
		slog.Info("github oauth web flow enabled", "callback_url", cfg.GitHub.CallbackURL)
	} else {
		slog.Info("github oauth web flow not configured (github.client_id, client_secret, callback_url) - /api/v1/auth/github answers 501")
	}

	// --- LLM Key Service ---
	llmKeySecret := cfg.Auth.LLMKeyEncryptionSecret
	if llmKeySecret == "" {
		llmKeySecret = cfg.Auth.JWTSecret
		if cfg.AppEnv != "" && cfg.AppEnv != "development" {
			slog.Warn("LLM key encryption uses the JWT secret - set CODEFORGE_AUTH_LLM_KEY_ENCRYPTION_SECRET (or _FILE) for production so rotating the JWT secret does not make stored LLM keys unreadable")
		}
	}
	llmKeyEncKey, err := crypto.DeriveKey(llmKeySecret, nil, "codeforge/llmkey/v1")
	if err != nil {
		return fmt.Errorf("derive llm key encryption key: %w", err)
	}
	llmKeySvc := service.NewLLMKeyService(store, llmKeyEncKey)

	// --- MCP Service (Phase 15C) ---
	mcpSvc := service.NewMCPService(&cfg.MCP, &cfg.Limits)
	mcpSvc.SetStore(store)
	runtimeSvc.SetMCPService(mcpSvc)
	slog.Info("mcp service initialized", "enabled", cfg.MCP.Enabled)

	// --- Copilot Token Exchange ---
	var copilotClient *copilot.Client
	if cfg.Copilot.Enabled {
		copilotClient = copilot.NewClient(cfg.Copilot.HostsFilePath)
		if copilotClient.HasHostsFile() {
			slog.Info("copilot integration enabled", "hosts_file", cfg.Copilot.HostsFilePath)
		} else {
			slog.Warn("copilot enabled but hosts file not found — exchange will fail until file is created")
		}
	}

	// --- Subscription Providers (OAuth device flow) ---
	envPath := filepath.Join(cfg.Workspace.Root, "..", ".env")
	if cfg.EnvFile != "" {
		envPath = cfg.EnvFile
	}
	subscriptionSvc := service.NewSubscriptionService(envPath,
		cfauth.NewAnthropicProvider(),
		cfauth.NewGitHubProvider(cfauth.WithClientID(cfg.GitHub.ClientID)),
	)
	slog.Info("subscription provider service initialized", "env_path", envPath)

	// --- Model Registry (Phase 22) ---
	// Periodic polling of LiteLLM model health; first refresh is synchronous.
	modelRegistry := service.NewModelRegistry(llmClient, hub, cfg.LiteLLM.HealthPollInterval, cfg.Ollama.BaseURL)
	modelRegistry.SetRoutingService(routingSvc)
	modelRegistry.Start(ctx)
	modelRegistry.ValidateConfiguredModel(cfg.LiteLLM.ConversationModel)
	slog.Info("model registry initialized",
		"poll_interval", cfg.LiteLLM.HealthPollInterval,
		"best_model", modelRegistry.BestModel(),
		"model_count", len(modelRegistry.AvailableModels()))

	// Wire model registry into services that need dynamic model resolution.
	metaAgentSvc.SetModelRegistry(modelRegistry)
	retrievalSvc.SetModelRegistry(modelRegistry)
	contextOptSvc.SetModelRegistry(modelRegistry)

	// --- Conversation Service ---
	// Use the static config model as fallback; the registry provides live best model.
	conversationModel := cfg.LiteLLM.ConversationModel
	convMsgSvc := service.NewConversationMessageService(store, queue, hub)
	conversationSvc := service.NewConversationService(store, hub, conversationModel, modeSvc)
	conversationSvc.SetToolUIDs(toolUIDSvc)
	conversationSvc.SetMessageService(convMsgSvc)
	conversationSvc.SetMetrics(metrics)
	conversationSvc.SetQueue(queue)
	conversationSvc.SetRunTracker(runtimeSvc)
	conversationSvc.SetAgentConfig(&cfg.Agent)
	conversationSvc.SetRuntimeConfig(&cfg.Runtime)
	conversationSvc.SetMCPService(mcpSvc)
	conversationSvc.SetPolicyService(policySvc)
	conversationSvc.SetModelRegistry(modelRegistry)
	conversationSvc.SetRoutingConfig(&cfg.Routing)
	conversationSvc.SetAppEnv(cfg.AppEnv)
	conversationSvc.SetContextOptimizer(contextOptSvc)
	conversationSvc.SetLLMKeyService(llmKeySvc)
	promptLib, plErr := service.NewPromptLibraryService(service.PromptsFS(), "prompts")
	if plErr != nil {
		slog.Error("failed to load modular prompt library, using legacy template fallback", "error", plErr)
	} else {
		assembler := service.NewPromptAssembler(promptLib, 0)

		// Wire prompt evolution: selector overrides base YAML with evolved variants.
		evoCfg := prompt.DefaultEvolutionConfig()
		promptSelector := service.NewPromptSelector(store, &evoCfg)
		assembler.SetSelector(promptSelector)

		conversationSvc.SetPromptAssembler(assembler)
		conversationSvc.SetEventStore(eventStore)

		// Wire PromptAssemblyService sub-service.
		// goalSvc is nil here; it is wired later via SetGoalService.
		promptSvc := service.NewPromptAssemblyService(store, contextOptSvc, nil, modeSvc, assembler, eventStore, &cfg.Agent)
		promptSvc.SetAppEnv(cfg.AppEnv)
		promptSvc.SetLLMKeyService(llmKeySvc)
		conversationSvc.SetPromptService(promptSvc)
		slog.Info("modular prompt library loaded", "entries", promptLib.Len())
	}

	// --- Prompt Evolution Service ---
	evoCfg := prompt.DefaultEvolutionConfig()
	evoSvc := service.NewPromptEvolutionService(queue, store, &evoCfg)
	scoreCollector := service.NewPromptScoreCollector(store)
	slog.Info("prompt score collector initialized")
	conversationSvc.SetPromptScoreCollector(scoreCollector)
	convRunCancel, err := conversationSvc.StartCompletionSubscriber(ctx)
	if err != nil {
		return fmt.Errorf("conversation run subscriber: %w", err)
	}
	convDeadLetterCancel, err := conversationSvc.StartDeadLetterSubscriber(ctx)
	if err != nil {
		return fmt.Errorf("conversation dead-letter subscriber: %w", err)
	}
	convCompactCancel, err := conversationSvc.StartCompactSubscriber(ctx)
	if err != nil {
		return fmt.Errorf("conversation compact subscriber: %w", err)
	}
	evoCancels, err := evoSvc.StartSubscribers(ctx)
	if err != nil {
		return fmt.Errorf("prompt evolution subscribers: %w", err)
	}
	gemmasCancel, err := evalSvc.StartGemmasResultSubscriber(ctx)
	if err != nil {
		return fmt.Errorf("gemmas result subscriber: %w", err)
	}
	slog.Info("conversation service initialized", "agentic_by_default", cfg.Agent.AgenticByDefault)

	// --- Auto-Agent Service ---
	autoAgentSvc := service.NewAutoAgentService(store, hub, queue, conversationSvc)
	autoAgentSvc.SetToolUIDs(toolUIDSvc)
	// Every feature is verified (KI-152); the checks run in the worker (KI-81).
	autoAgentSvc.SetVerification(service.AutoAgentVerification{
		FixAttempts:        cfg.Agent.AutoAgentFixAttempts,
		ToolOutputMaxChars: cfg.Agent.ToolOutputMaxChars,
		Defaults:           project.GateCommands{Test: cfg.Runtime.DefaultTestCommand, Lint: cfg.Runtime.DefaultLintCommand},
	})
	autoAgentTestCancel, err := autoAgentSvc.StartTestResultSubscriber(ctx)
	if err != nil {
		return fmt.Errorf("auto-agent test result subscriber: %w", err)
	}
	slog.Info("auto-agent service initialized")

	// --- Workspace deletions through the worker (KI-96 D11) ---
	cancelWorkspaceDeletions, stopWorkspaceDeletionRetry := func() {}, func() {}
	if toolUIDSvc.Required() {
		cancelWorkspaceDeletions, err = workspaceDeletionSvc.StartSubscribers(ctx)
		if err != nil {
			return fmt.Errorf("workspace deletion subscribers: %w", err)
		}
		stopWorkspaceDeletionRetry = workspaceDeletionSvc.StartRetryJob(ctx)
		slog.Info("workspace deletions run in the worker as the tenants' tool UIDs")
	}

	// --- Auth Service (Phase 10C) ---
	authSvc := service.NewAuthService(store, &cfg.Auth)
	if cfg.Auth.Enabled {
		if err := authSvc.BootstrapAdmin(context.Background(), middleware.DefaultTenantID); err != nil {
			slog.Warn("failed to bootstrap admin", "error", err)
		}
		// Without users the setup page needs the one-time setup token (KI-119).
		// Fails closed: without an armed token the setup is refused.
		if _, err := authSvc.PrepareSetupToken(context.Background(), middleware.DefaultTenantID); err != nil {
			slog.Warn("failed to prepare the setup token", "error", err)
		}
		// Warn if initial password file still exists (should be changed on first login)
		if cfg.Auth.InitialPasswordFile != "" {
			if _, err := os.Stat(cfg.Auth.InitialPasswordFile); err == nil { //nolint:gosec // path from trusted config
				slog.Warn("initial admin password file exists — change the password and delete this file",
					"file", cfg.Auth.InitialPasswordFile)
			}
		}
		// Start background cleanup of expired revoked tokens (P1-6)
		authSvc.StartTokenCleanup(ctx, 15*time.Minute)
	}
	// An erased user's access tokens stop working on this replica at once (KI-143).
	gdprSvc := service.NewGDPRService(store)
	gdprSvc.SetTokenInvalidator(authSvc.Tokens())

	// --- Memory Service (Phase 22B) ---
	memorySvc := service.NewMemoryService(store, queue)
	memoryCancels, err := memorySvc.StartSubscribers(ctx)
	if err != nil {
		return fmt.Errorf("memory subscribers: %w", err)
	}
	slog.Info("memory service initialized", "subscribers", len(memoryCancels))

	// --- Experience Pool Service (Phase 22B) ---
	experienceSvc := service.NewExperiencePoolService(store)
	slog.Info("experience pool service initialized")

	// --- Microagent Service (Phase 22C) ---
	microagentSvc := service.NewMicroagentService(store)
	runtimeSvc.SetMicroagentService(microagentSvc)
	conversationSvc.SetMicroagentService(microagentSvc)
	// Auto-load microagents from .codeforge/microagents/ directory.
	if agents, loadErr := microagent.LoadFromDirectory(".codeforge/microagents"); loadErr != nil {
		slog.Warn("failed to load microagents from .codeforge/microagents", "error", loadErr)
	} else if len(agents) > 0 {
		loaded := 0
		for _, ma := range agents {
			if _, err := microagentSvc.Create(ctx, &microagent.CreateRequest{
				Name:           ma.Name,
				Type:           ma.Type,
				TriggerPattern: ma.TriggerPattern,
				Description:    ma.Description,
				Prompt:         ma.Prompt,
			}); err != nil {
				slog.Debug("skip microagent (already exists or invalid)", "name", ma.Name, "error", err)
				continue
			}
			loaded++
		}
		slog.Info("microagents auto-loaded from .codeforge/microagents", "loaded", loaded, "total", len(agents))
	}
	slog.Info("microagent service initialized")

	// --- Goal Discovery Service (Phase 28) ---
	goalSvc := service.NewGoalDiscoveryService(store)
	projectSvc.SetGoalDiscovery(goalSvc)
	conversationSvc.SetGoalService(goalSvc)
	runtimeSvc.SetGoalService(goalSvc)
	// Wire goal service into prompt assembly sub-service (created earlier with nil goalSvc).
	if conversationSvc.PromptService() != nil {
		conversationSvc.PromptService().SetGoalService(goalSvc)
	}
	contextOptSvc.SetGoalService(goalSvc)
	runtimeSvc.SetGoalService(goalSvc)
	slog.Info("goal discovery service initialized")

	// --- Skill Service (Phase 22D) ---
	skillSvc := service.NewSkillService(store)
	slog.Info("skill service initialized")

	// --- File Service ---
	fileSvc := service.NewFileService(store)
	slog.Info("file service initialized")

	// --- Feedback Providers (Phase 22D) ---
	// The operator's approval channels receive the requests of
	// notification.approval_tenants only (KI-84).
	if slackFB, why := slackApprovalProvider(&cfg.Notification); slackFB != nil {
		runtimeSvc.RegisterFeedbackProvider(slackFB)
		slog.Info("slack feedback provider registered",
			"tenants", cfg.Notification.ApprovalTenants, "web_ui_url", cfg.Notification.WebUIURL)
	} else {
		slog.Info(why)
	}
	if emailFB, why := emailApprovalProvider(&cfg.Notification); emailFB != nil {
		runtimeSvc.RegisterFeedbackProvider(emailFB)
		slog.Info("email feedback provider registered",
			"recipients", len(cfg.Notification.ApprovalRecipients), "tenants", cfg.Notification.ApprovalTenants,
			"web_ui_url", cfg.Notification.WebUIURL)
	} else {
		slog.Info(why)
	}

	benchmarkSuiteSvc := service.NewBenchmarkSuiteService(store, cfg.Benchmark.DatasetsDir)
	benchmarkRunMgr := service.NewBenchmarkRunManager(store, benchmarkSuiteSvc)
	benchmarkRunMgr.SetToolUIDs(toolUIDSvc)
	benchmarkResultAgg := service.NewBenchmarkResultAggregator(store)
	benchmarkWatchdog := service.NewBenchmarkWatchdog(store)
	benchmarkSvc := service.NewBenchmarkService(benchmarkSuiteSvc, benchmarkRunMgr, benchmarkResultAgg, benchmarkWatchdog)
	benchmarkSvc.SetRoutingService(routingSvc)
	benchmarkSvc.SetQueue(queue)
	benchmarkSvc.SetHub(hub)
	benchmarkRunCancel, err := benchmarkSvc.StartResultSubscriber(ctx)
	if err != nil {
		return fmt.Errorf("benchmark run subscriber: %w", err)
	}
	benchmarkSvc.SeedDefaultSuites(ctx)
	cancelWatchdog := benchmarkSvc.StartWatchdog(5*time.Minute, cfg.Benchmark.WatchdogTimeout)
	slog.Info("benchmark service initialized with NATS bridge", "watchdog_timeout", cfg.Benchmark.WatchdogTimeout)

	// --- Backend Health Service (Phase 5.4) ---
	backendHealthSvc := service.NewBackendHealthService(queue)
	backendHealthCancel, err := queue.Subscribe(ctx, messagequeue.SubjectBackendHealthResult, func(_ context.Context, _ string, data []byte) error {
		return backendHealthSvc.HandleHealthResult(ctx, data)
	})
	if err != nil {
		return fmt.Errorf("backend health subscriber: %w", err)
	}
	slog.Info("backend health service initialized")

	handlers := &cfhttp.Handlers{
		Projects:         projectSvc,
		Tasks:            taskSvc,
		Agents:           agentSvc,
		LLM:              llmClient,
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
		Sync:             syncSvc,
		Webhooks:         webhookSvc,
		Notification:     notificationSvc,
		Auth:             authSvc,
		Scope:            scopeSvc,
		Pipelines:        pipelineSvc,
		Review:           reviewSvc,
		KnowledgeBases:   kbSvc,
		Settings:         settingsSvc,
		VCSAccounts:      vcsAccountSvc,
		GitHubOAuth:      githubOAuthSvc,
		LLMKeys:          llmKeySvc,
		Conversations:    conversationSvc,
		LSP:              lspSvc,
		MCP:              mcpSvc,
		PromptSections:   service.NewPromptSectionService(store),
		Benchmarks:       benchmarkSvc,
		ReviewRouter:     reviewRouterSvc,
		ModelRegistry:    modelRegistry,
		TokenExchanger:   copilotClient,
		Memory:           memorySvc,
		ExperiencePool:   experienceSvc,
		Microagents:      microagentSvc,
		Skills:           skillSvc,
		Files:            fileSvc,
		Quarantine:       quarantineSvc,
		ActiveWork:       activeWorkSvc,
		Routing:          routingSvc,
		GoalDiscovery:    goalSvc,
		Dashboard:        dashboardSvc,
		AutoAgent:        autoAgentSvc,
		BackendHealth:    backendHealthSvc,
		Checkpoint:       checkpointSvc,
		Commands:         service.NewCommandService(),
		Subscription:     subscriptionSvc,
		Channels:         service.NewChannelService(store, hub),
		Limits:           &cfg.Limits,
		AgentConfig:      &cfg.Agent,
		AppEnv:           cfg.AppEnv,
		OllamaBaseURL:    cfg.Ollama.BaseURL,
		Boundaries:       boundarySvc,
		ReviewTrigger:    reviewTriggerSvc,
		ReviewPipeline:   reviewPipelineSvc,
		PromptEvolution:  evoSvc,
		GDPR:             gdprSvc,
		Consent:          service.NewConsentService(store),
		WSTickets:        wsTickets,
	}
	handlers.WireGroups()

	// A2A Client Service (Phase 27K + 27O) — outbound federation + push notifications.
	if cfg.A2A.Enabled {
		a2aSvc := service.NewA2AService(store, queue, hub)
		handlers.A2A = a2aSvc
		// Handoffs to remote agents go through the A2A client.
		handoffSvc.SetA2AService(a2aSvc)

		a2aCompletionCancel, a2aSubErr := a2aSvc.StartCompletionSubscriber(ctx)
		if a2aSubErr != nil {
			return fmt.Errorf("a2a completion subscriber: %w", a2aSubErr)
		}
		defer a2aCompletionCancel()

		slog.Info("a2a client service enabled", "completion_subscriber", true)
	}

	// The handoff subscribers start once every dependency of the handoff
	// service is wired (the A2A client above): a handler must not read a
	// dependency while it is still being set.
	cancelHandoffs, err := handoffSvc.StartSubscribers(ctx)
	if err != nil {
		return fmt.Errorf("handoff subscribers: %w", err)
	}

	// A2A API keys with their tenants (validated at config load).
	a2aAPIKeys, err := a2aKeys(&cfg.A2A)
	if err != nil {
		return err
	}

	r := chi.NewRouter()

	// Rate limiter
	rateLimiter := middleware.NewRateLimiter(cfg.Rate.RequestsPerSecond, cfg.Rate.Burst)
	rateLimiterCleanup := rateLimiter.StartCleanup(cfg.Rate.CleanupInterval, cfg.Rate.MaxIdleTime)
	defer rateLimiterCleanup()

	// Client IP first so logging, auditing and rate limiting see the real client
	// (forwarding headers only from server.trusted_proxies, KI-11).
	trustedProxies, err := cfg.Server.TrustedProxyPrefixes()
	if err != nil {
		return fmt.Errorf("server.trusted_proxies: %w", err)
	}
	r.Use(middleware.ClientIP(trustedProxies))

	// Middleware (applied to all routes including WebSocket)
	r.Use(cfhttp.SecurityHeaders)
	r.Use(cfhttp.CORS(cfg.Server.CORSOrigin, cfg.AppEnv))
	if cfg.OTEL.Enabled {
		r.Use(cfotel.HTTPMiddleware(cfg.OTEL.ServiceName))
	}
	r.Use(middleware.RequestID)
	r.Use(middleware.Auth(authSvc, cfg.Auth.Enabled, cfg.InternalKey))
	r.Use(middleware.TenantID)
	r.Use(cfhttp.Logger)
	r.Use(chimw.Recoverer)

	// WebSocket — rate-limited but no Timeout/Idempotency (long-lived connection)
	r.Group(func(wsGroup chi.Router) {
		wsGroup.Use(rateLimiter.Handler)
		wsGroup.Get("/ws", hub.HandleWS)
	})

	// Health endpoints — rate-limited to prevent amplification attacks.
	// readinessHandler performs 3 I/O ops per call (PG ping, NATS check, LiteLLM check).
	r.Group(func(health chi.Router) {
		health.Use(rateLimiter.Handler)
		health.Get("/health", livenessHandler(logDropped, cfg.AppEnv))
		health.Get("/health/ready", readinessHandler(pool, queue, llmClient))
	})

	// API routes wrapped in a group with additional middleware
	r.Group(func(api chi.Router) {
		api.Use(chimw.Timeout(30 * time.Second))
		api.Use(rateLimiter.Handler)
		api.Use(middleware.Idempotency(idempotencyKV))

		// FIX-084: Stricter rate limiter for auth endpoints (brute-force protection).
		authRL := middleware.NewRateLimiter(cfg.Rate.AuthPerSecond, cfg.Rate.AuthBurst)
		authRLCleanup := authRL.StartCleanup(cfg.Rate.CleanupInterval, cfg.Rate.MaxIdleTime)
		defer authRLCleanup()

		cfhttp.MountRoutes(api, handlers, cfhttp.WithAuthRateLimiter(authRL))

		// A2A protocol routes (Phase 27 — SDK-based)
		if cfg.A2A.Enabled {
			// Build mode list for AgentCard skills.
			allModes := modeSvc.List()
			modeInfos := make([]cfa2a.ModeInfo, 0, len(allModes))
			modeIDs := make([]string, 0, len(allModes))
			for i := range allModes {
				modeInfos = append(modeInfos, cfa2a.ModeInfo{
					ID: allModes[i].ID, Name: allModes[i].Name, Description: allModes[i].Description,
				})
				modeIDs = append(modeIDs, allModes[i].ID)
			}

			// Base URL for AgentCard.
			a2aBaseURL := cfg.A2A.BaseURL
			if a2aBaseURL == "" {
				a2aBaseURL = "http://localhost:" + cfg.Server.Port
			}

			// Build components.
			cardBuilder := cfa2a.NewCardBuilder(a2aBaseURL, modeInfos, cfversion.Version,
				cfa2a.WithStreaming(cfg.A2A.Streaming))
			taskStoreAdapter := cfa2a.NewTaskStoreAdapter(store)
			executor := cfa2a.NewExecutor(store, queue, hub, modeIDs)
			if cfg.Quarantine.Enabled {
				// Inbound A2A prompts are screened before any worker sees them (KI-15).
				executor.SetScreener(quarantineSvc)
			}

			// Wire SDK handler.
			a2aReqHandler := a2asrv.NewHandler(executor,
				a2asrv.WithTaskStore(taskStoreAdapter),
				a2asrv.WithExtendedAgentCardProducer(cardBuilder),
			)
			a2aHTTPHandler := a2asrv.NewJSONRPCHandler(a2aReqHandler)

			// A2A callers authenticate with A2A API keys (not a user's JWT)
			// and act in their key's tenant (KI-15).
			if len(a2aAPIKeys) == 0 {
				slog.Warn("a2a enabled without a2a.api_keys: every A2A request is refused")
			}
			a2aAuth := middleware.A2AAuth(a2aAPIKeys)
			r.Group(func(r chi.Router) {
				r.Use(a2aAuth)
				r.Handle("/a2a", a2aHTTPHandler)
			})
			// AgentCard discovery — gated by AllowOpen config.
			if cfg.A2A.AllowOpen {
				r.Get("/.well-known/agent-card.json", a2asrv.NewAgentCardHandler(cardBuilder).ServeHTTP)
			} else {
				r.Group(func(r chi.Router) {
					r.Use(a2aAuth)
					r.Get("/.well-known/agent-card.json", a2asrv.NewAgentCardHandler(cardBuilder).ServeHTTP)
				})
			}

			slog.Info("a2a protocol enabled", "transport", cfg.A2A.Transport, "base_url", a2aBaseURL)
		}
	})

	// --- MCP Server (Phase 15B) ---
	var mcpServer *cfmcp.Server
	if cfg.MCP.Enabled {
		var mcpErr error
		mcpServer, mcpErr = cfmcp.NewServer(cfmcp.ServerConfig{
			Addr:    fmt.Sprintf(":%d", cfg.MCP.ServerPort),
			Name:    "codeforge",
			Version: cfversion.Version,
			APIKey:  cfg.MCP.APIKey,
		}, cfmcp.ServerDeps{
			ProjectLister: store,
			RunReader:     store,
			CostReader:    store,
		})
		if mcpErr != nil {
			return fmt.Errorf("mcp server init: %w", mcpErr)
		}
		if err := mcpServer.Start(); err != nil {
			slog.Warn("mcp server failed to start, MCP integration unavailable",
				"port", cfg.MCP.ServerPort,
				"error", err)
			mcpServer = nil
		} else {
			slog.Info("mcp server started", "port", cfg.MCP.ServerPort)
		}
	}

	addr := ":" + cfg.Server.Port

	srv := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: cfg.Server.ReadHeaderTimeout,
		ReadTimeout:       cfg.Server.ReadTimeout,
		WriteTimeout:      cfg.Server.WriteTimeout,
		IdleTimeout:       cfg.Server.IdleTimeout,
		MaxHeaderBytes:    1 << 13,
	}

	// Wait for interrupt signal
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	// SIGHUP reloads the secrets vault and names the changed settings that
	// need a restart (see reloadOnSIGHUP).
	sighup := make(chan os.Signal, 1)
	signal.Notify(sighup, syscall.SIGHUP)
	go func() {
		for range sighup {
			reloadOnSIGHUP(cfg, flags, vault)
		}
	}()

	go func() {
		slog.Info("starting server", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server failed", "error", err)
		}
	}()

	// --- Stuck-work watchdog (KI-28 quality gates, KI-65 lost workers, KI-33 teams) ---
	// Work acked on accept whose worker stopped sending heartbeats is ended
	// through its completion path (runtime.heartbeat_timeout; 0 disables).
	// Teams whose plans all ended are ended as well.
	endedTeams := service.StuckWorkCheck{Name: "ended teams", EndStuck: func(ctx context.Context) (int, error) {
		return poolManagerSvc.CleanupEndedTeams(ctx, store)
	}}
	// Refactorings whose plan ended but whose worker never confirmed the stop
	// (or that ended while Go Core was down) are measured after the grace.
	undecidedRefactorings := service.StuckWorkCheck{Name: "undecided review refactorings", EndStuck: func(ctx context.Context) (int, error) {
		return reviewPipelineSvc.EndUndecidedRefactorings(ctx, store)
	}}
	// Teams whose plans ended while Go Core was down end at startup.
	service.NewStuckWorkWatchdog(0, endedTeams, undecidedRefactorings).RunOnce(ctx)
	stopStuckWorkWatchdog := service.NewStuckWorkWatchdog(cfg.Runtime.StaleCheckInterval,
		service.StuckWorkCheck{Name: "lost tasks", EndStuck: func(ctx context.Context) (int, error) {
			return agentSvc.FailTasksWithLostWorker(ctx, service.LostWorkerAfter(&cfg.Runtime))
		}},
		service.StuckWorkCheck{Name: "tasks never accepted", EndStuck: func(ctx context.Context) (int, error) {
			return agentSvc.FailTasksNeverAccepted(ctx, cfg.Runtime.TaskAcceptTimeout)
		}},
		service.StuckWorkCheck{Name: "quality gates", EndStuck: runtimeSvc.FailStuckQualityGates},
		service.StuckWorkCheck{Name: "lost runs", EndStuck: runtimeSvc.EndRunsWithLostWorker},
		service.StuckWorkCheck{Name: "lost conversation runs", EndStuck: conversationSvc.EndConversationRunsWithLostWorker},
		endedTeams,
		undecidedRefactorings,
		// KI-91: held messages past their review deadline expire, and the
		// inbound A2A task waiting for one is rejected with it.
		service.StuckWorkCheck{Name: "expired quarantine messages", EndStuck: quarantineSvc.ExpireOverdue},
	).Start(ctx)

	// --- Data retention (GDPR Art. 5(1)(e), docs/data-retention.md) ---
	// Its own daily ticker: the stuck-work watchdog ticks every
	// stale_check_interval and reports what it ends as stuck work.
	stopRetention := service.NewRetentionService(store, cfg.Retention, cfg.Webhook.DeliveryRetention).Start(ctx)

	<-done

	// --- Ordered Graceful Shutdown ---
	// Phase 1: Stop accepting new HTTP requests
	slog.Info("shutdown phase 1: stopping HTTP server")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("http shutdown error", "error", err)
	}
	if mcpServer != nil {
		if err := mcpServer.Stop(shutdownCtx); err != nil {
			slog.Error("mcp server shutdown error", "error", err)
		}
	}

	// Phase 2: Cancel NATS subscribers and background tasks
	slog.Info("shutdown phase 2: cancelling NATS subscribers")
	stopStuckWorkWatchdog()
	stopRetention()
	stopWorkspaceDeletionRetry()
	cancelWorkspaceDeletions()
	for _, cancel := range runtimeCancels {
		cancel()
	}
	cancelResults()
	cancelOutput()
	cancelAgentOutput()
	cancelTaskHeartbeats()
	cancelTaskDeadLetters()
	cancelHandoffs()
	repoMapCancel()
	convRunCancel()
	convDeadLetterCancel()
	autoAgentTestCancel()
	convCompactCancel()
	for _, cancel := range evoCancels {
		cancel()
	}
	gemmasCancel()
	benchmarkRunCancel()
	cancelWatchdog()
	for _, cancel := range retrievalCancels {
		cancel()
	}
	for _, cancel := range graphCancels {
		cancel()
	}
	for _, cancel := range memoryCancels {
		cancel()
	}
	backendHealthCancel()

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
// It also includes a dev_mode flag so the frontend can conditionally
// show development-only features like the benchmark page, and dropped_logs
// to surface async logger buffer pressure.
func livenessHandler(dropped logger.DroppedCounter, appEnv string) http.HandlerFunc {
	devMode := appEnv == "development"
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(struct {
			Status      string `json:"status"`
			DevMode     bool   `json:"dev_mode"`
			DroppedLogs int64  `json:"dropped_logs"`
		}{
			Status:      "ok",
			DevMode:     devMode,
			DroppedLogs: dropped.DroppedCount(),
		})
	}
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

// a2aKeys returns the A2A API keys of an enabled A2A server (nil for a
// disabled one, whose keys config load does not validate either).
func a2aKeys(cfg *config.A2A) ([]config.A2AAPIKey, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	keys, err := cfg.ParsedAPIKeys()
	if err != nil {
		return nil, fmt.Errorf("a2a.api_keys: %w", err)
	}
	return keys, nil
}
