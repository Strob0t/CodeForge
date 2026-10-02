package http_test

import (
	"context"
	"encoding/json"
	"errors"
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
	"github.com/Strob0t/CodeForge/internal/domain/mcp"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/middleware"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/service"
)

// auditStoreMock implements the auditDB interface (middleware.AuditStore + auditLogReader).
type auditStoreMock struct {
	entries   []database.AuditEntry
	listErr   error
	inserted  []database.AuditEntry
	insertErr error
}

// InsertAuditEntry refuses what PostgreSQL refuses: a NUL byte in a text column.
func (m *auditStoreMock) InsertAuditEntry(_ context.Context, e *database.AuditEntry) error {
	if m.insertErr != nil {
		return m.insertErr
	}
	if strings.ContainsRune(e.ResourceID, 0) || strings.Contains(string(e.Details), `\u0000`) {
		return errors.New("ERROR: invalid byte sequence for encoding \"UTF8\": 0x00 (SQLSTATE 22021)")
	}
	m.inserted = append(m.inserted, *e)
	return nil
}

func (m *auditStoreMock) ListAuditEntries(_ context.Context, _ string, _, _ int) ([]database.AuditEntry, error) {
	return m.entries, m.listErr
}

// newAuditTestRouter creates a chi router with the audit store wired in and a
// configurable user injected into the request context. When ctxUser is nil the
// default admin is used (matching the convention in other handler tests).
func newAuditTestRouter(auditStore *auditStoreMock, ctxUser *user.User) chi.Router {
	return newAuditTestRouterWithStore(auditStore, ctxUser, &mockStore{})
}

// newAuditTestRouterWithStore is newAuditTestRouter on *store*.
func newAuditTestRouterWithStore(auditStore *auditStoreMock, ctxUser *user.User, store *mockStore) chi.Router {
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
	mcpSvc := newTestMCPService(store)
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
	mountTestRoutes(r, handlers, cfhttp.WithAuditStore(auditStore))
	return r
}

func TestAuditLogs_Admin_ReturnsJSONArray(t *testing.T) {
	now := time.Now().UTC()
	adminEmail := "admin@example.com"
	auditStore := &auditStoreMock{
		entries: []database.AuditEntry{
			{
				ID:         "ae-1",
				TenantID:   "t1",
				AdminID:    "admin-1",
				AdminEmail: &adminEmail,
				Action:     "create",
				Resource:   "project",
				ResourceID: "p-1",
				CreatedAt:  now,
			},
			{
				ID:         "ae-2",
				TenantID:   "t1",
				AdminID:    "admin-1",
				AdminEmail: &adminEmail,
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
	if entries[0].AdminEmail == nil || *entries[0].AdminEmail != adminEmail {
		t.Errorf("first entry admin_email = %v, want %q", entries[0].AdminEmail, adminEmail)
	}
}

// KI-53: an entry whose admin was erased (GDPR) has no email; the listing
// returns it with admin_email null and without ip_address.
func TestAuditLogs_ErasedAdmin_EmailIsNull(t *testing.T) {
	auditStore := &auditStoreMock{
		entries: []database.AuditEntry{{
			ID: "ae-erased", TenantID: "t1", AdminID: "admin-gone",
			Action: "delete", Resource: "user", CreatedAt: time.Now().UTC(),
		}},
	}

	r := newAuditTestRouter(auditStore, nil)
	req := httptest.NewRequest("GET", "/api/v1/audit-logs", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var entries []map[string]json.RawMessage
	if err := json.NewDecoder(w.Body).Decode(&entries); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if got, ok := entries[0]["admin_email"]; !ok || string(got) != "null" {
		t.Errorf("admin_email = %s (present %v), want null", got, ok)
	}
	if got, ok := entries[0]["ip_address"]; ok {
		t.Errorf("ip_address = %s, want it omitted", got)
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

// mcpAssignmentStore has project p1 and the MCP servers s1 and decoy.
func mcpAssignmentStore() *mockStore {
	return &mockStore{
		projects: []project.Project{{ID: "p1", Name: "p"}},
		mcpServers: []mcp.ServerDef{
			{ID: "s1", Name: "s1", Transport: mcp.TransportSSE, URL: "http://s1.example/sse"},
			{ID: "decoy", Name: "decoy", Transport: mcp.TransportSSE, URL: "http://decoy.example/sse"},
		},
	}
}

func auditLines(entries []database.AuditEntry) []string {
	var lines []string
	for i := range entries {
		e := &entries[i]
		lines = append(lines, e.Action+" "+e.Resource+" "+e.ResourceID+" "+string(e.Details))
	}
	return lines
}

// KI-71 review: assigning an MCP server to a project, or removing it, is
// audited as an action on that server, with its project. The entry names
// what the handler decoded and acts on, never a second reading of the body
// the requester could make differ from it.
func TestAudit_MCPServerAssignmentsRecordWhatIsAssigned(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"plain", `{"server_id":"s1"}`},
		{"trailing data after the object", `{"server_id":"s1"} {"server_id":"decoy"}`},
		{"case-insensitive duplicate key", `{"server_id":"decoy","SERVER_ID":"s1"}`},
		{"padding over 64 KiB", `{"pad":"` + strings.Repeat("x", 70_000) + `","server_id":"s1"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			auditStore := &auditStoreMock{}
			store := mcpAssignmentStore()
			r := newAuditTestRouterWithStore(auditStore, nil, store)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/p1/mcp-servers", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != http.StatusNoContent {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
			if len(store.mcpProjectLinks) != 1 || store.mcpProjectLinks[0].ServerID != "s1" {
				t.Fatalf("links = %+v, want p1 -> s1", store.mcpProjectLinks)
			}
			want := []string{`assign mcp_server s1 {"project_id":"p1"}`}
			if got := auditLines(auditStore.inserted); strings.Join(got, "|") != strings.Join(want, "|") {
				t.Fatalf("audit entries = %q, want %q", got, want)
			}
		})
	}
}

func TestAudit_MCPServerUnassignRecordsServerAndProject(t *testing.T) {
	auditStore := &auditStoreMock{}
	store := mcpAssignmentStore()
	store.mcpProjectLinks = append(store.mcpProjectLinks, struct{ ProjectID, ServerID string }{"p1", "s1"})
	r := newAuditTestRouterWithStore(auditStore, nil, store)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/api/v1/projects/p1/mcp-servers/s1", http.NoBody))

	if w.Code != http.StatusNoContent {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	want := []string{`unassign mcp_server s1 {"project_id":"p1"}`}
	if got := auditLines(auditStore.inserted); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("audit entries = %q, want %q", got, want)
	}
}

// A NUL byte in a decoy value made the insert fail (PostgreSQL text), so no
// entry was written: the stored values are made storable.
func TestAudit_MCPServerAssignmentWithANulByteIsRecorded(t *testing.T) {
	auditStore := &auditStoreMock{}
	r := newAuditTestRouterWithStore(auditStore, nil, mcpAssignmentStore())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/p1/mcp-servers", strings.NewReader(`{"server_id":"s1\u0000decoy"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(httptest.NewRecorder(), req)

	if len(auditStore.inserted) != 1 || auditStore.inserted[0].ResourceID != "s1\uFFFDdecoy" {
		t.Fatalf("audit entries = %q, want one for the server with the NUL replaced", auditLines(auditStore.inserted))
	}
}

// The entry is written before the change; an assignment whose entry cannot
// be written is refused, so no assignment goes unaudited.
func TestAudit_MCPServerAssignmentIsRefusedWithoutItsAuditEntry(t *testing.T) {
	auditStore := &auditStoreMock{insertErr: errors.New("audit_log unavailable")}
	store := mcpAssignmentStore()
	store.mcpProjectLinks = append(store.mcpProjectLinks, struct{ ProjectID, ServerID string }{"p1", "decoy"})
	r := newAuditTestRouterWithStore(auditStore, nil, store)

	assign := httptest.NewRequest(http.MethodPost, "/api/v1/projects/p1/mcp-servers", strings.NewReader(`{"server_id":"s1"}`))
	assign.Header.Set("Content-Type", "application/json")
	for _, req := range []*http.Request{assign, httptest.NewRequest(http.MethodDelete, "/api/v1/projects/p1/mcp-servers/decoy", http.NoBody)} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s %s: status %d, want 503: %s", req.Method, req.URL.Path, w.Code, w.Body.String())
		}
	}
	if len(store.mcpProjectLinks) != 1 || store.mcpProjectLinks[0].ServerID != "decoy" {
		t.Fatalf("links = %+v, want only the existing p1 -> decoy", store.mcpProjectLinks)
	}
}

// A request refused before anything changed is still audited, with its status.
func TestAudit_MCPServerAssignmentRefusedEarlyIsRecorded(t *testing.T) {
	auditStore := &auditStoreMock{}
	r := newAuditTestRouterWithStore(auditStore, nil, mcpAssignmentStore())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/p1/mcp-servers", strings.NewReader(`not json`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(httptest.NewRecorder(), req)

	want := []string{`assign mcp_server  {"project_id":"p1","status":"400"}`}
	if got := auditLines(auditStore.inserted); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("audit entries = %q, want %q", got, want)
	}
}
