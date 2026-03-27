package http_test

import (
	"context"
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
	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/middleware"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/service"
)

// auditStoreMock implements the auditDB interface (middleware.AuditStore + auditLogReader).
type auditStoreMock struct {
	entries []database.AuditEntry
	listErr error
}

func (m *auditStoreMock) InsertAuditEntry(_ context.Context, _ *database.AuditEntry) error {
	return nil
}

func (m *auditStoreMock) ListAuditEntries(_ context.Context, _ string, _, _ int) ([]database.AuditEntry, error) {
	return m.entries, m.listErr
}

// newAuditTestRouter creates a chi router with the audit store wired in and a
// configurable user injected into the request context. When ctxUser is nil the
// default admin is used (matching the convention in other handler tests).
func newAuditTestRouter(auditStore *auditStoreMock, ctxUser *user.User) chi.Router {
	store := &mockStore{}
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
		ActiveWork:    service.NewActiveWorkService(store, bc),
		Routing:       service.NewRoutingService(store),
		GoalDiscovery: service.NewGoalDiscoveryService(store, osfs.New()),
		AppEnv:        os.Getenv("APP_ENV"),
		Limits: &config.Limits{
			MaxRequestBodySize: 1 << 20,
			MaxQueryLength:     2000,
			MaxFiles:           50,
			MaxFileSize:        32768,
			MaxInputLen:        10000,
			MaxEntries:         100,
		},
	}

	if ctxUser == nil {
		ctxUser = &user.User{
			ID:   "test-admin",
			Name: "Test Admin",
			Role: user.RoleAdmin,
		}
	}

	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.URL.Path, "/api/v1/auth/") {
				if middleware.UserFromContext(r.Context()) == nil {
					r = r.WithContext(middleware.ContextWithTestUser(r.Context(), ctxUser))
				}
			}
			next.ServeHTTP(w, r)
		})
	})
	cfhttp.MountRoutes(r, handlers, config.Webhook{}, cfhttp.WithAuditStore(auditStore))
	return r
}

func TestAuditLogs_Admin_ReturnsJSONArray(t *testing.T) {
	now := time.Now().UTC()
	auditStore := &auditStoreMock{
		entries: []database.AuditEntry{
			{
				ID:         "ae-1",
				TenantID:   "t1",
				AdminID:    "admin-1",
				AdminEmail: "admin@example.com",
				Action:     "create",
				Resource:   "project",
				ResourceID: "p-1",
				CreatedAt:  now,
			},
			{
				ID:         "ae-2",
				TenantID:   "t1",
				AdminID:    "admin-1",
				AdminEmail: "admin@example.com",
				Action:     "delete",
				Resource:   "user",
				ResourceID: "u-1",
				CreatedAt:  now,
			},
		},
	}

	r := newAuditTestRouter(auditStore, nil) // nil = default admin
	req := httptest.NewRequest("GET", "/api/v1/audit-logs", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var entries []database.AuditEntry
	if err := json.NewDecoder(w.Body).Decode(&entries); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].ID != "ae-1" {
		t.Errorf("first entry ID = %q, want %q", entries[0].ID, "ae-1")
	}
	if entries[1].Action != "delete" {
		t.Errorf("second entry action = %q, want %q", entries[1].Action, "delete")
	}
}

func TestAuditLogs_Admin_EmptyList(t *testing.T) {
	auditStore := &auditStoreMock{entries: nil}

	r := newAuditTestRouter(auditStore, nil)
	req := httptest.NewRequest("GET", "/api/v1/audit-logs", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var entries []database.AuditEntry
	if err := json.NewDecoder(w.Body).Decode(&entries); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected empty array, got %d entries", len(entries))
	}
}

func TestAuditLogs_ViewerRole_Forbidden(t *testing.T) {
	auditStore := &auditStoreMock{}
	viewer := &user.User{
		ID:   "viewer-001",
		Name: "Viewer User",
		Role: user.RoleViewer,
	}

	r := newAuditTestRouter(auditStore, viewer)
	req := httptest.NewRequest("GET", "/api/v1/audit-logs", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAuditLogs_EditorRole_Forbidden(t *testing.T) {
	auditStore := &auditStoreMock{}
	editor := &user.User{
		ID:   "editor-001",
		Name: "Editor User",
		Role: user.RoleEditor,
	}

	r := newAuditTestRouter(auditStore, editor)
	req := httptest.NewRequest("GET", "/api/v1/audit-logs", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAuditLogs_WithActionFilter(t *testing.T) {
	auditStore := &auditStoreMock{
		entries: []database.AuditEntry{
			{
				ID:     "ae-filtered",
				Action: "login",
			},
		},
	}

	r := newAuditTestRouter(auditStore, nil)
	req := httptest.NewRequest("GET", "/api/v1/audit-logs?action=login", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var entries []database.AuditEntry
	if err := json.NewDecoder(w.Body).Decode(&entries); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].ID != "ae-filtered" {
		t.Errorf("entry ID = %q, want %q", entries[0].ID, "ae-filtered")
	}
}
