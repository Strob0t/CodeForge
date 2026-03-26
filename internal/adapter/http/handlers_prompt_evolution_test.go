package http_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	cfhttp "github.com/Strob0t/CodeForge/internal/adapter/http"
	"github.com/Strob0t/CodeForge/internal/adapter/litellm"
	"github.com/Strob0t/CodeForge/internal/adapter/osfs"
	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/prompt"
	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/middleware"
	"github.com/Strob0t/CodeForge/internal/service"
)

// newTestRouterWithPromptEvolution creates a test router with PromptEvolution service wired.
func newTestRouterWithPromptEvolution(store *mockStore) chi.Router {
	queue := &mockQueue{}
	bc := &mockBroadcaster{}
	es := &mockEventStore{}
	policySvc := service.NewPolicyService("headless-safe-sandbox", nil)
	runtimeSvc := service.NewRuntimeService(store, queue, bc, es, policySvc, &config.Runtime{})
	orchCfg := &config.Orchestrator{
		MaxParallel:       4,
		PingPongMaxRounds: 3,
		MaxTeamSize:       5,
	}
	orchSvc := service.NewOrchestratorService(store, bc, es, runtimeSvc, orchCfg)
	poolManagerSvc := service.NewPoolManagerService(store, bc, orchCfg)
	metaAgentSvc := service.NewMetaAgentService(store, litellm.NewClient("http://localhost:4000", ""), orchSvc, orchCfg, &config.Limits{})
	taskPlannerSvc := service.NewTaskPlannerService(metaAgentSvc, poolManagerSvc, store, orchCfg, &config.Limits{})
	contextOptSvc := service.NewContextOptimizerService(store, osfs.New(), orchCfg, &config.Limits{})
	sharedCtxSvc := service.NewSharedContextService(store, bc, queue)
	modeSvc := service.NewModeService()
	pipelineSvc := service.NewPipelineService(modeSvc)
	repoMapSvc := service.NewRepoMapService(store, queue, bc, orchCfg)
	retrievalSvc := service.NewRetrievalService(store, queue, bc, orchCfg, &config.Limits{})
	costSvc := service.NewCostService(store)
	settingsSvc := service.NewSettingsService(store)
	vcsAccountSvc := service.NewVCSAccountService(store, []byte("test-encryption-key-32bytes!!!!!"))
	conversationSvc := service.NewConversationService(store, bc, "", nil)
	conversationSvc.SetQueue(queue)
	authCfg := &config.Auth{
		Enabled:            true,
		JWTSecret:          "test-secret-key-32bytes-handler!",
		AccessTokenExpiry:  15 * time.Minute,
		RefreshTokenExpiry: 7 * 24 * time.Hour,
		BcryptCost:         4,
	}
	authSvc := service.NewAuthService(store, authCfg)
	filesSvc := service.NewFileService(store, osfs.New())
	roadmapSvc := service.NewRoadmapService(store, bc, nil, nil)
	autoAgentSvc := service.NewAutoAgentService(store, bc, queue, conversationSvc)
	microagentSvc := service.NewMicroagentService(store)
	skillSvc := service.NewSkillService(store)
	memorySvc := service.NewMemoryService(store, queue)
	experiencePoolSvc := service.NewExperiencePoolService(store)
	kbSvc := service.NewKnowledgeBaseService(store)
	sessionSvc := service.NewSessionService(store, es)
	mcpSvc := service.NewMCPService(&config.MCP{}, &config.Limits{MCPTestTimeout: 10 * time.Second})
	mcpSvc.SetStore(store)

	// Build prompt evolution service.
	evoCfg := prompt.DefaultEvolutionConfig()
	evoSvc := service.NewPromptEvolutionService(queue, nil, &evoCfg)

	handlers := &cfhttp.Handlers{
		Projects:         service.NewProjectService(store, os.TempDir()),
		Tasks:            service.NewTaskService(store, queue),
		Agents:           service.NewAgentService(store, queue, bc),
		LLM:              litellm.NewClient("http://localhost:4000", ""),
		Policies:         policySvc,
		Runtime:          runtimeSvc,
		Orchestrator:     orchSvc,
		MetaAgent:        metaAgentSvc,
		PoolManager:      poolManagerSvc,
		TaskPlanner:      taskPlannerSvc,
		ContextOptimizer: contextOptSvc,
		SharedContext:    sharedCtxSvc,
		Modes:            modeSvc,
		Pipelines:        pipelineSvc,
		RepoMap:          repoMapSvc,
		Retrieval:        retrievalSvc,
		Events:           es,
		Cost:             costSvc,
		Settings:         settingsSvc,
		VCSAccounts:      vcsAccountSvc,
		Conversations:    conversationSvc,
		Auth:             authSvc,
		Files:            filesSvc,
		Roadmap:          roadmapSvc,
		AutoAgent:        autoAgentSvc,
		Microagents:      microagentSvc,
		Skills:           skillSvc,
		Memory:           memorySvc,
		ExperiencePool:   experiencePoolSvc,
		KnowledgeBases:   kbSvc,
		Sessions:         sessionSvc,
		MCP:              mcpSvc,
		Scope:            service.NewScopeService(store),
		PromptSections:   service.NewPromptSectionService(store),
		Benchmarks: func() *service.BenchmarkService {
			suiteSvc := service.NewBenchmarkSuiteService(store, os.TempDir())
			runMgr := service.NewBenchmarkRunManager(store, suiteSvc)
			resultAgg := service.NewBenchmarkResultAggregator(store)
			watchdog := service.NewBenchmarkWatchdog(store)
			return service.NewBenchmarkService(suiteSvc, runMgr, resultAgg, watchdog)
		}(),
		ActiveWork:      service.NewActiveWorkService(store, bc),
		Routing:         service.NewRoutingService(store),
		GoalDiscovery:   service.NewGoalDiscoveryService(store, osfs.New()),
		PromptEvolution: evoSvc,
		AppEnv:          os.Getenv("APP_ENV"),
		Limits: &config.Limits{
			MaxRequestBodySize: 1 << 20,
			MaxQueryLength:     2000,
			MaxFiles:           50,
			MaxFileSize:        32768,
			MaxInputLen:        10000,
			MaxEntries:         100,
		},
	}

	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.URL.Path, "/api/v1/auth/") {
				if middleware.UserFromContext(r.Context()) == nil {
					r = r.WithContext(middleware.ContextWithTestUser(r.Context(), &user.User{
						ID:   "test-admin",
						Name: "Test Admin",
						Role: user.RoleAdmin,
					}))
				}
			}
			next.ServeHTTP(w, r)
		})
	})
	cfhttp.MountRoutes(r, handlers, config.Webhook{})
	return r
}

func TestTriggerPromptEvolutionReflect_Success(t *testing.T) {
	t.Parallel()

	r := newTestRouterWithPromptEvolution(&mockStore{})

	body := `{
		"mode_id": "coder",
		"model_family": "openai",
		"current_prompt": "You are a coding assistant.",
		"failures": [{"task_id": "t1", "error": "failed to handle edge case"}]
	}`
	req := httptest.NewRequest("POST", "/api/v1/prompt-evolution/reflect", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTriggerPromptEvolutionReflect_MissingModeID(t *testing.T) {
	t.Parallel()

	r := newTestRouterWithPromptEvolution(&mockStore{})

	body := `{
		"model_family": "openai",
		"current_prompt": "prompt"
	}`
	req := httptest.NewRequest("POST", "/api/v1/prompt-evolution/reflect", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing mode_id, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTriggerPromptEvolutionReflect_MissingModelFamily(t *testing.T) {
	t.Parallel()

	r := newTestRouterWithPromptEvolution(&mockStore{})

	body := `{
		"mode_id": "coder",
		"current_prompt": "prompt"
	}`
	req := httptest.NewRequest("POST", "/api/v1/prompt-evolution/reflect", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing model_family, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTriggerPromptEvolutionReflect_MissingCurrentPrompt(t *testing.T) {
	t.Parallel()

	r := newTestRouterWithPromptEvolution(&mockStore{})

	body := `{
		"mode_id": "coder",
		"model_family": "openai"
	}`
	req := httptest.NewRequest("POST", "/api/v1/prompt-evolution/reflect", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing current_prompt, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTriggerPromptEvolutionReflect_NilService(t *testing.T) {
	t.Parallel()

	// Use default test router which does NOT set PromptEvolution.
	r := newTestRouter()
	body := `{
		"mode_id": "coder",
		"model_family": "openai",
		"current_prompt": "prompt"
	}`
	req := httptest.NewRequest("POST", "/api/v1/prompt-evolution/reflect", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Route is only mounted when PromptEvolution != nil, so we get 405 (chi returns 405 for known paths).
	if w.Code != http.StatusMethodNotAllowed && w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 or 405 when evolution service is nil, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetPromptEvolutionStatus_WithService(t *testing.T) {
	t.Parallel()

	r := newTestRouterWithPromptEvolution(&mockStore{})

	req := httptest.NewRequest("GET", "/api/v1/prompt-evolution/status", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var status prompt.EvolutionStatus
	if err := json.NewDecoder(w.Body).Decode(&status); err != nil {
		t.Fatalf("failed to decode status: %v", err)
	}
	if !status.Enabled {
		t.Error("expected enabled=true")
	}
}
