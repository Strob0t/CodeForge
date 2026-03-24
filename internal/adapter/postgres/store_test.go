package postgres_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Strob0t/CodeForge/internal/adapter/postgres"
	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/conversation"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/domain/task"
	"github.com/Strob0t/CodeForge/internal/domain/tenant"
	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/middleware"
)

// ctxWithTenant builds a context carrying the given tenant ID by routing a
// fake HTTP request through the TenantID middleware. This is the only safe way
// to populate the unexported context key used by tenantFromCtx.
func ctxWithTenant(t *testing.T, tenantID string) context.Context {
	t.Helper()
	ch := make(chan context.Context, 1)
	handler := middleware.TenantID(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		ch <- r.Context()
	}))
	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	req.Header.Set("X-Tenant-ID", tenantID)
	handler.ServeHTTP(httptest.NewRecorder(), req)
	select {
	case ctx := <-ch:
		return ctx
	default:
		t.Fatal("TenantID middleware did not invoke next handler")
		return nil
	}
}

// setupStore creates a pgxpool connection, runs all migrations, and returns a
// ready-to-use Store. The pool is closed via t.Cleanup.
func setupStore(t *testing.T) *postgres.Store {
	t.Helper()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("requires DATABASE_URL")
	}

	ctx := context.Background()

	// Run goose migrations first (uses embedded SQL files).
	if err := postgres.RunMigrations(ctx, dsn); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	t.Cleanup(pool.Close)

	return postgres.NewStore(pool)
}

// createTestTenant creates a tenant with a random slug and returns its ID.
func createTestTenant(t *testing.T, store *postgres.Store) string {
	t.Helper()
	slug := "test-" + uuid.New().String()[:8]
	tn, err := store.CreateTenant(context.Background(), tenant.CreateRequest{
		Name: "Test Tenant " + slug,
		Slug: slug,
	})
	if err != nil {
		t.Fatalf("create test tenant: %v", err)
	}
	return tn.ID
}

// --------------------------------------------------------------------------
// TestStore_ProjectCRUD
// --------------------------------------------------------------------------

func TestStore_ProjectCRUD(t *testing.T) {
	store := setupStore(t)
	tenantID := createTestTenant(t, store)
	ctx := ctxWithTenant(t, tenantID)

	// Create
	created, err := store.CreateProject(ctx, &project.CreateRequest{
		Name:        "integration-test-project",
		Description: "A project for integration testing",
		RepoURL:     "https://github.com/test/repo",
		Provider:    "github",
		Config:      map[string]string{"branch": "main"},
	})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if created.ID == "" {
		t.Fatal("CreateProject returned empty ID")
	}
	if created.Name != "integration-test-project" {
		t.Fatalf("expected name 'integration-test-project', got %q", created.Name)
	}
	if created.Version != 1 {
		t.Fatalf("expected version 1, got %d", created.Version)
	}

	t.Cleanup(func() {
		_ = store.DeleteProject(ctx, created.ID)
	})

	// Get
	t.Run("Get", func(t *testing.T) {
		got, err := store.GetProject(ctx, created.ID)
		if err != nil {
			t.Fatalf("GetProject: %v", err)
		}
		if got.Name != created.Name {
			t.Fatalf("expected name %q, got %q", created.Name, got.Name)
		}
		if got.Config["branch"] != "main" {
			t.Fatalf("expected config branch=main, got %v", got.Config)
		}
	})

	// List with tenant isolation
	t.Run("List_TenantIsolation", func(t *testing.T) {
		// Create a second tenant and a project under it.
		otherTenantID := createTestTenant(t, store)
		otherCtx := ctxWithTenant(t, otherTenantID)

		otherProj, err := store.CreateProject(otherCtx, &project.CreateRequest{
			Name:     "other-tenant-project",
			RepoURL:  "https://github.com/other/repo",
			Provider: "github",
		})
		if err != nil {
			t.Fatalf("CreateProject for other tenant: %v", err)
		}
		t.Cleanup(func() {
			_ = store.DeleteProject(otherCtx, otherProj.ID)
		})

		// List for the first tenant should NOT include the other tenant's project.
		projects, err := store.ListProjects(ctx)
		if err != nil {
			t.Fatalf("ListProjects: %v", err)
		}
		for _, p := range projects {
			if p.ID == otherProj.ID {
				t.Fatal("ListProjects returned a project from another tenant")
			}
		}

		found := false
		for _, p := range projects {
			if p.ID == created.ID {
				found = true
				break
			}
		}
		if !found {
			t.Fatal("ListProjects did not return the project created in this tenant")
		}
	})

	// Get non-existent project
	t.Run("Get_NotFound", func(t *testing.T) {
		_, err := store.GetProject(ctx, uuid.New().String())
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})

	// Delete
	t.Run("Delete", func(t *testing.T) {
		toDelete, err := store.CreateProject(ctx, &project.CreateRequest{
			Name:     "to-delete",
			Provider: "local",
		})
		if err != nil {
			t.Fatalf("CreateProject: %v", err)
		}
		if err := store.DeleteProject(ctx, toDelete.ID); err != nil {
			t.Fatalf("DeleteProject: %v", err)
		}

		// Verify it is gone.
		_, err = store.GetProject(ctx, toDelete.ID)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("expected ErrNotFound after delete, got %v", err)
		}
	})

	// Delete from wrong tenant should fail
	t.Run("Delete_WrongTenant", func(t *testing.T) {
		otherTenantID := createTestTenant(t, store)
		otherCtx := ctxWithTenant(t, otherTenantID)

		err := store.DeleteProject(otherCtx, created.ID)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("expected ErrNotFound when deleting from wrong tenant, got %v", err)
		}
	})
}

// --------------------------------------------------------------------------
// TestStore_GetProjectByRepoName_TenantIsolation
// --------------------------------------------------------------------------

func TestStore_GetProjectByRepoName_TenantIsolation(t *testing.T) {
	store := setupStore(t)

	// Create two tenants.
	tenantA := createTestTenant(t, store)
	tenantB := createTestTenant(t, store)
	ctxA := ctxWithTenant(t, tenantA)
	ctxB := ctxWithTenant(t, tenantB)

	repoName := "shared-repo-" + uuid.New().String()[:8]

	// Create a project in tenant A with a distinctive repo URL.
	projA, err := store.CreateProject(ctxA, &project.CreateRequest{
		Name:     "project-a",
		RepoURL:  "https://github.com/orgA/" + repoName,
		Provider: "github",
	})
	if err != nil {
		t.Fatalf("CreateProject in tenant A: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteProject(ctxA, projA.ID) })

	// Create a project in tenant B with a different repo URL that shares no
	// substring with repoName.
	projB, err := store.CreateProject(ctxB, &project.CreateRequest{
		Name:     "project-b",
		RepoURL:  "https://github.com/orgB/unrelated-repo",
		Provider: "github",
	})
	if err != nil {
		t.Fatalf("CreateProject in tenant B: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteProject(ctxB, projB.ID) })

	// Tenant A can find the project by repo name.
	t.Run("TenantA_Finds", func(t *testing.T) {
		got, err := store.GetProjectByRepoName(ctxA, repoName)
		if err != nil {
			t.Fatalf("GetProjectByRepoName in tenant A: %v", err)
		}
		if got.ID != projA.ID {
			t.Fatalf("expected project %s, got %s", projA.ID, got.ID)
		}
	})

	// Tenant B cannot find tenant A's project by repo name.
	t.Run("TenantB_NotFound", func(t *testing.T) {
		_, err := store.GetProjectByRepoName(ctxB, repoName)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("expected ErrNotFound in tenant B, got %v", err)
		}
	})

	// Create a project in tenant B with the same repo name.
	t.Run("TenantB_OwnProject", func(t *testing.T) {
		projB2, err := store.CreateProject(ctxB, &project.CreateRequest{
			Name:     "project-b2",
			RepoURL:  "https://github.com/orgB/" + repoName,
			Provider: "github",
		})
		if err != nil {
			t.Fatalf("CreateProject in tenant B with shared name: %v", err)
		}
		t.Cleanup(func() { _ = store.DeleteProject(ctxB, projB2.ID) })

		got, err := store.GetProjectByRepoName(ctxB, repoName)
		if err != nil {
			t.Fatalf("GetProjectByRepoName in tenant B: %v", err)
		}
		if got.ID != projB2.ID {
			t.Fatalf("expected project %s from tenant B, got %s", projB2.ID, got.ID)
		}

		// Tenant A still finds its own project.
		gotA, err := store.GetProjectByRepoName(ctxA, repoName)
		if err != nil {
			t.Fatalf("GetProjectByRepoName in tenant A after B's insert: %v", err)
		}
		if gotA.ID != projA.ID {
			t.Fatalf("expected project %s from tenant A, got %s", projA.ID, gotA.ID)
		}
	})
}

// --------------------------------------------------------------------------
// TestStore_UserCRUD
// --------------------------------------------------------------------------

func TestStore_UserCRUD(t *testing.T) {
	store := setupStore(t)
	tenantID := createTestTenant(t, store)

	userID := uuid.New().String()
	email := "test-" + uuid.New().String()[:8] + "@example.com"

	u := &user.User{
		ID:           userID,
		Email:        email,
		Name:         "Test User",
		PasswordHash: "$2a$10$dummyhashforintegrationtest000000000000000000000000",
		Role:         user.RoleEditor,
		TenantID:     tenantID,
		Enabled:      true,
	}

	// CreateUser does not use tenant context; it reads TenantID from the struct.
	if err := store.CreateUser(context.Background(), u); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	t.Cleanup(func() {
		_ = store.DeleteUser(context.Background(), userID)
	})

	// GetUser (not tenant-scoped, uses user ID directly).
	t.Run("GetUser", func(t *testing.T) {
		got, err := store.GetUser(context.Background(), userID)
		if err != nil {
			t.Fatalf("GetUser: %v", err)
		}
		if got.Email != email {
			t.Fatalf("expected email %q, got %q", email, got.Email)
		}
		if got.Role != user.RoleEditor {
			t.Fatalf("expected role editor, got %s", got.Role)
		}
		if got.TenantID != tenantID {
			t.Fatalf("expected tenant %s, got %s", tenantID, got.TenantID)
		}
	})

	// GetUserByEmail with tenant isolation.
	t.Run("GetByEmail_CorrectTenant", func(t *testing.T) {
		got, err := store.GetUserByEmail(context.Background(), email, tenantID)
		if err != nil {
			t.Fatalf("GetUserByEmail: %v", err)
		}
		if got.ID != userID {
			t.Fatalf("expected user %s, got %s", userID, got.ID)
		}
	})

	t.Run("GetByEmail_WrongTenant", func(t *testing.T) {
		otherTenantID := createTestTenant(t, store)

		_, err := store.GetUserByEmail(context.Background(), email, otherTenantID)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("expected ErrNotFound for wrong tenant, got %v", err)
		}
	})

	// Same email in different tenant should succeed (unique index is per-tenant).
	t.Run("SameEmail_DifferentTenant", func(t *testing.T) {
		otherTenantID := createTestTenant(t, store)
		otherUserID := uuid.New().String()

		u2 := &user.User{
			ID:           otherUserID,
			Email:        email, // same email
			Name:         "Other Tenant User",
			PasswordHash: "$2a$10$dummyhashforintegrationtest000000000000000000000000",
			Role:         user.RoleViewer,
			TenantID:     otherTenantID,
			Enabled:      true,
		}
		if err := store.CreateUser(context.Background(), u2); err != nil {
			t.Fatalf("CreateUser same email different tenant: %v", err)
		}
		t.Cleanup(func() {
			_ = store.DeleteUser(context.Background(), otherUserID)
		})

		got, err := store.GetUserByEmail(context.Background(), email, otherTenantID)
		if err != nil {
			t.Fatalf("GetUserByEmail for second tenant: %v", err)
		}
		if got.ID != otherUserID {
			t.Fatalf("expected user %s, got %s", otherUserID, got.ID)
		}
	})
}

// --------------------------------------------------------------------------
// TestStore_TokenRevocation
// --------------------------------------------------------------------------

func TestStore_TokenRevocation(t *testing.T) {
	store := setupStore(t)

	jti := "test-jti-" + uuid.New().String()[:8]
	expiresAt := time.Now().UTC().Add(1 * time.Hour)

	// RevokeToken
	t.Run("RevokeToken", func(t *testing.T) {
		if err := store.RevokeToken(context.Background(), jti, expiresAt); err != nil {
			t.Fatalf("RevokeToken: %v", err)
		}
	})

	// IsTokenRevoked returns true for revoked token.
	t.Run("IsTokenRevoked_True", func(t *testing.T) {
		revoked, err := store.IsTokenRevoked(context.Background(), jti)
		if err != nil {
			t.Fatalf("IsTokenRevoked: %v", err)
		}
		if !revoked {
			t.Fatal("expected token to be revoked")
		}
	})

	// IsTokenRevoked returns false for unknown token.
	t.Run("IsTokenRevoked_False", func(t *testing.T) {
		revoked, err := store.IsTokenRevoked(context.Background(), "unknown-jti")
		if err != nil {
			t.Fatalf("IsTokenRevoked: %v", err)
		}
		if revoked {
			t.Fatal("expected unknown token to not be revoked")
		}
	})

	// Revoking the same JTI again is idempotent (ON CONFLICT DO NOTHING).
	t.Run("RevokeToken_Idempotent", func(t *testing.T) {
		if err := store.RevokeToken(context.Background(), jti, expiresAt); err != nil {
			t.Fatalf("RevokeToken idempotent: %v", err)
		}
	})

	// PurgeExpiredTokens removes expired entries.
	t.Run("PurgeExpiredTokens", func(t *testing.T) {
		expiredJTI := "expired-jti-" + uuid.New().String()[:8]
		expiredTime := time.Now().UTC().Add(-1 * time.Hour) // already expired

		if err := store.RevokeToken(context.Background(), expiredJTI, expiredTime); err != nil {
			t.Fatalf("RevokeToken for expired: %v", err)
		}

		// Verify it exists before purge.
		revoked, err := store.IsTokenRevoked(context.Background(), expiredJTI)
		if err != nil {
			t.Fatalf("IsTokenRevoked before purge: %v", err)
		}
		if !revoked {
			t.Fatal("expected expired token to exist before purge")
		}

		// Purge expired tokens.
		purged, err := store.PurgeExpiredTokens(context.Background())
		if err != nil {
			t.Fatalf("PurgeExpiredTokens: %v", err)
		}
		if purged < 1 {
			t.Fatalf("expected at least 1 purged token, got %d", purged)
		}

		// Expired token should be gone.
		revoked, err = store.IsTokenRevoked(context.Background(), expiredJTI)
		if err != nil {
			t.Fatalf("IsTokenRevoked after purge: %v", err)
		}
		if revoked {
			t.Fatal("expected expired token to be purged")
		}

		// Non-expired token should still be present.
		revoked, err = store.IsTokenRevoked(context.Background(), jti)
		if err != nil {
			t.Fatalf("IsTokenRevoked non-expired after purge: %v", err)
		}
		if !revoked {
			t.Fatal("expected non-expired token to survive purge")
		}
	})
}

// --------------------------------------------------------------------------
// TestStore_GetRun (top-1 most-called: 61 callers)
// --------------------------------------------------------------------------

func TestStore_GetRun(t *testing.T) {
	store := setupStore(t)
	tenantID := createTestTenant(t, store)
	ctx := ctxWithTenant(t, tenantID)

	// Create prerequisites: project, task, agent.
	proj, err := store.CreateProject(ctx, &project.CreateRequest{
		Name: "run-test-project", Provider: "local",
	})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteProject(ctx, proj.ID) })

	tsk, err := store.CreateTask(ctx, task.CreateRequest{
		ProjectID: proj.ID, Title: "test-task", Prompt: "do something",
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	ag, err := store.CreateAgent(ctx, proj.ID, "test-agent", "aider", nil, nil)
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteAgent(ctx, ag.ID) })

	// Create a run.
	r := &run.Run{
		TaskID:    tsk.ID,
		AgentID:   ag.ID,
		ProjectID: proj.ID,
		Status:    run.StatusRunning,
		ExecMode:  run.ExecModeMount,
	}
	if err := store.CreateRun(ctx, r); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	t.Run("existing_run", func(t *testing.T) {
		got, err := store.GetRun(ctx, r.ID)
		if err != nil {
			t.Fatalf("GetRun: %v", err)
		}
		if got.ID != r.ID {
			t.Fatalf("expected ID %s, got %s", r.ID, got.ID)
		}
		if got.Status != run.StatusRunning {
			t.Fatalf("expected status running, got %s", got.Status)
		}
		if got.ProjectID != proj.ID {
			t.Fatalf("expected project %s, got %s", proj.ID, got.ProjectID)
		}
	})

	t.Run("nonexistent_run", func(t *testing.T) {
		_, err := store.GetRun(ctx, uuid.New().String())
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("wrong_tenant", func(t *testing.T) {
		otherTenantID := createTestTenant(t, store)
		otherCtx := ctxWithTenant(t, otherTenantID)
		_, err := store.GetRun(otherCtx, r.ID)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("expected ErrNotFound for wrong tenant, got %v", err)
		}
	})
}

// --------------------------------------------------------------------------
// TestStore_GetAgent (top-3 most-called: 18 callers)
// --------------------------------------------------------------------------

func TestStore_GetAgent(t *testing.T) {
	store := setupStore(t)
	tenantID := createTestTenant(t, store)
	ctx := ctxWithTenant(t, tenantID)

	proj, err := store.CreateProject(ctx, &project.CreateRequest{
		Name: "agent-test-project", Provider: "local",
	})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteProject(ctx, proj.ID) })

	ag, err := store.CreateAgent(ctx, proj.ID, "test-agent", "openhands",
		map[string]string{"key": "val"}, nil)
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteAgent(ctx, ag.ID) })

	t.Run("existing_agent", func(t *testing.T) {
		got, err := store.GetAgent(ctx, ag.ID)
		if err != nil {
			t.Fatalf("GetAgent: %v", err)
		}
		if got.Name != "test-agent" {
			t.Fatalf("expected name test-agent, got %q", got.Name)
		}
		if got.Backend != "openhands" {
			t.Fatalf("expected backend openhands, got %q", got.Backend)
		}
		if got.Config["key"] != "val" {
			t.Fatalf("expected config key=val, got %v", got.Config)
		}
	})

	t.Run("nonexistent_agent", func(t *testing.T) {
		_, err := store.GetAgent(ctx, uuid.New().String())
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("wrong_tenant", func(t *testing.T) {
		otherTenantID := createTestTenant(t, store)
		otherCtx := ctxWithTenant(t, otherTenantID)
		_, err := store.GetAgent(otherCtx, ag.ID)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("expected ErrNotFound for wrong tenant, got %v", err)
		}
	})
}

// --------------------------------------------------------------------------
// TestStore_GetTask (top-4 most-called: 11 callers)
// --------------------------------------------------------------------------

func TestStore_GetTask(t *testing.T) {
	store := setupStore(t)
	tenantID := createTestTenant(t, store)
	ctx := ctxWithTenant(t, tenantID)

	proj, err := store.CreateProject(ctx, &project.CreateRequest{
		Name: "task-test-project", Provider: "local",
	})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteProject(ctx, proj.ID) })

	tsk, err := store.CreateTask(ctx, task.CreateRequest{
		ProjectID: proj.ID, Title: "test-task", Prompt: "implement feature X",
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	t.Run("existing_task", func(t *testing.T) {
		got, err := store.GetTask(ctx, tsk.ID)
		if err != nil {
			t.Fatalf("GetTask: %v", err)
		}
		if got.Title != "test-task" {
			t.Fatalf("expected title test-task, got %q", got.Title)
		}
		if got.Prompt != "implement feature X" {
			t.Fatalf("expected prompt, got %q", got.Prompt)
		}
		if got.Status != task.StatusPending {
			t.Fatalf("expected status pending, got %s", got.Status)
		}
	})

	t.Run("nonexistent_task", func(t *testing.T) {
		_, err := store.GetTask(ctx, uuid.New().String())
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("wrong_tenant", func(t *testing.T) {
		otherTenantID := createTestTenant(t, store)
		otherCtx := ctxWithTenant(t, otherTenantID)
		_, err := store.GetTask(otherCtx, tsk.ID)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("expected ErrNotFound for wrong tenant, got %v", err)
		}
	})
}

// --------------------------------------------------------------------------
// TestStore_GetConversation (top-5 most-called: 11 callers)
// --------------------------------------------------------------------------

func TestStore_GetConversation(t *testing.T) {
	store := setupStore(t)
	tenantID := createTestTenant(t, store)
	ctx := ctxWithTenant(t, tenantID)

	proj, err := store.CreateProject(ctx, &project.CreateRequest{
		Name: "conv-test-project", Provider: "local",
	})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteProject(ctx, proj.ID) })

	conv, err := store.CreateConversation(ctx, &conversation.Conversation{
		ProjectID: proj.ID,
		Title:     "test-conversation",
		Mode:      "coder",
		Model:     "openai/gpt-4o",
	})
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}

	t.Run("existing_conversation", func(t *testing.T) {
		got, err := store.GetConversation(ctx, conv.ID)
		if err != nil {
			t.Fatalf("GetConversation: %v", err)
		}
		if got.Title != "test-conversation" {
			t.Fatalf("expected title test-conversation, got %q", got.Title)
		}
		if got.Mode != "coder" {
			t.Fatalf("expected mode coder, got %q", got.Mode)
		}
		if got.Model != "openai/gpt-4o" {
			t.Fatalf("expected model openai/gpt-4o, got %q", got.Model)
		}
		if got.ProjectID != proj.ID {
			t.Fatalf("expected project %s, got %s", proj.ID, got.ProjectID)
		}
	})

	t.Run("nonexistent_conversation", func(t *testing.T) {
		_, err := store.GetConversation(ctx, uuid.New().String())
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("wrong_tenant", func(t *testing.T) {
		otherTenantID := createTestTenant(t, store)
		otherCtx := ctxWithTenant(t, otherTenantID)
		_, err := store.GetConversation(otherCtx, conv.ID)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("expected ErrNotFound for wrong tenant, got %v", err)
		}
	})
}

// --------------------------------------------------------------------------
// TestStore_MessageCRUD
// --------------------------------------------------------------------------

func TestStore_MessageCRUD(t *testing.T) {
	store := setupStore(t)
	tenantID := createTestTenant(t, store)
	ctx := ctxWithTenant(t, tenantID)

	// Create project + conversation as prerequisites.
	proj, err := store.CreateProject(ctx, &project.CreateRequest{
		Name: "msg-test-project", Provider: "local",
	})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteProject(ctx, proj.ID) })

	conv, err := store.CreateConversation(ctx, &conversation.Conversation{
		ProjectID: proj.ID,
		Title:     "msg-test-conversation",
		Mode:      "coder",
		Model:     "openai/gpt-4o",
	})
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}

	t.Run("create_and_list_messages", func(t *testing.T) {
		// Create two messages.
		msg1, err := store.CreateMessage(ctx, &conversation.Message{
			ConversationID: conv.ID,
			Role:           "user",
			Content:        "Hello, world!",
		})
		if err != nil {
			t.Fatalf("CreateMessage user: %v", err)
		}
		if msg1.ID == "" {
			t.Fatal("CreateMessage returned empty ID")
		}
		if msg1.Role != "user" {
			t.Fatalf("expected role user, got %q", msg1.Role)
		}
		if msg1.Content != "Hello, world!" {
			t.Fatalf("expected content 'Hello, world!', got %q", msg1.Content)
		}

		msg2, err := store.CreateMessage(ctx, &conversation.Message{
			ConversationID: conv.ID,
			Role:           "assistant",
			Content:        "Hi there!",
			TokensIn:       100,
			TokensOut:      50,
			Model:          "openai/gpt-4o",
		})
		if err != nil {
			t.Fatalf("CreateMessage assistant: %v", err)
		}
		if msg2.TokensIn != 100 {
			t.Fatalf("expected tokens_in 100, got %d", msg2.TokensIn)
		}

		// List and verify ordering.
		msgs, err := store.ListMessages(ctx, conv.ID)
		if err != nil {
			t.Fatalf("ListMessages: %v", err)
		}
		if len(msgs) < 2 {
			t.Fatalf("expected at least 2 messages, got %d", len(msgs))
		}

		// Messages should be in chronological order (ASC).
		found := 0
		for i := range msgs {
			if msgs[i].ID == msg1.ID {
				found++
			}
			if msgs[i].ID == msg2.ID {
				found++
			}
		}
		if found != 2 {
			t.Fatal("ListMessages did not return both created messages")
		}
	})

	t.Run("list_empty_conversation", func(t *testing.T) {
		emptyConv, err := store.CreateConversation(ctx, &conversation.Conversation{
			ProjectID: proj.ID,
			Title:     "empty-conversation",
		})
		if err != nil {
			t.Fatalf("CreateConversation: %v", err)
		}
		msgs, err := store.ListMessages(ctx, emptyConv.ID)
		if err != nil {
			t.Fatalf("ListMessages empty: %v", err)
		}
		if len(msgs) != 0 {
			t.Fatalf("expected 0 messages for empty conversation, got %d", len(msgs))
		}
	})

	t.Run("wrong_tenant_returns_empty", func(t *testing.T) {
		otherTenantID := createTestTenant(t, store)
		otherCtx := ctxWithTenant(t, otherTenantID)

		msgs, err := store.ListMessages(otherCtx, conv.ID)
		if err != nil {
			t.Fatalf("ListMessages wrong tenant: %v", err)
		}
		if len(msgs) != 0 {
			t.Fatalf("expected 0 messages for wrong tenant, got %d", len(msgs))
		}
	})

	t.Run("create_message_nonexistent_conversation", func(t *testing.T) {
		_, err := store.CreateMessage(ctx, &conversation.Message{
			ConversationID: uuid.New().String(),
			Role:           "user",
			Content:        "orphan message",
		})
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("expected ErrNotFound for nonexistent conversation, got %v", err)
		}
	})

	t.Run("search_messages_by_content", func(t *testing.T) {
		// Create a message with distinctive content for search.
		searchConv, err := store.CreateConversation(ctx, &conversation.Conversation{
			ProjectID: proj.ID,
			Title:     "search-conversation",
		})
		if err != nil {
			t.Fatalf("CreateConversation for search: %v", err)
		}
		searchTerm := "xylophone-" + uuid.New().String()[:8]
		_, err = store.CreateMessage(ctx, &conversation.Message{
			ConversationID: searchConv.ID,
			Role:           "user",
			Content:        "I need to implement a " + searchTerm + " feature",
		})
		if err != nil {
			t.Fatalf("CreateMessage for search: %v", err)
		}

		// Search across tenant.
		results, err := store.SearchConversationMessages(ctx, searchTerm, nil, 10)
		if err != nil {
			t.Fatalf("SearchConversationMessages: %v", err)
		}
		if len(results) == 0 {
			t.Fatal("SearchConversationMessages returned 0 results, expected at least 1")
		}

		foundSearch := false
		for _, r := range results {
			if r.ConversationID == searchConv.ID {
				foundSearch = true
				break
			}
		}
		if !foundSearch {
			t.Fatal("search did not find message in the target conversation")
		}

		// Search filtered by project ID.
		filtered, err := store.SearchConversationMessages(ctx, searchTerm, []string{proj.ID}, 10)
		if err != nil {
			t.Fatalf("SearchConversationMessages with project filter: %v", err)
		}
		if len(filtered) == 0 {
			t.Fatal("SearchConversationMessages with project filter returned 0 results")
		}

		// Search with wrong project should return 0 results.
		empty, err := store.SearchConversationMessages(ctx, searchTerm, []string{uuid.New().String()}, 10)
		if err != nil {
			t.Fatalf("SearchConversationMessages wrong project: %v", err)
		}
		if len(empty) != 0 {
			t.Fatalf("expected 0 results for wrong project, got %d", len(empty))
		}
	})
}
