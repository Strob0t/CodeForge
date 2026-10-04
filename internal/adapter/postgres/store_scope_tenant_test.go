package postgres_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Strob0t/CodeForge/internal/adapter/postgres"
	"github.com/Strob0t/CodeForge/internal/domain"
	cfcontext "github.com/Strob0t/CodeForge/internal/domain/context"
	"github.com/Strob0t/CodeForge/internal/domain/project"
)

func createScopeTestProject(ctx context.Context, t *testing.T, store *postgres.Store) string {
	t.Helper()
	p, err := store.CreateProject(ctx, &project.CreateRequest{Name: "scope-" + uuid.NewString()[:8], Provider: "local"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	return p.ID
}

// TestStore_ScopeProjectsStayInTheTenant (S7-F review): a retrieval scope
// holds only projects of its own tenant, and only its tenant changes it. A
// scope search fans out to the scope's projects and the worker searches an
// index by project ID only, so another tenant's project in a scope would be
// searched in its name.
func TestStore_ScopeProjectsStayInTheTenant(t *testing.T) {
	store := setupStore(t)
	pool := retentionPool(t)
	ctxA := ctxWithTenant(t, createTestTenant(t, store))
	ctxB := ctxWithTenant(t, createTestTenant(t, store))
	projA, projB := createScopeTestProject(ctxA, t, store), createScopeTestProject(ctxB, t, store)
	projA2 := createScopeTestProject(ctxA, t, store)
	scopeB := createTestScope(ctxB, t, store)
	if err := store.AddProjectToScope(ctxB, scopeB.ID, projB); err != nil {
		t.Fatalf("AddProjectToScope(B): %v", err)
	}

	if _, err := store.CreateScope(ctxA, cfcontext.CreateScopeRequest{Name: "x", Type: cfcontext.ScopeShared, ProjectIDs: []string{projA, projB}}); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("CreateScope with another tenant's project = %v, want domain.ErrNotFound", err)
	}
	scopeA, err := store.CreateScope(ctxA, cfcontext.CreateScopeRequest{Name: "a-" + uuid.NewString()[:8], Type: cfcontext.ScopeShared, ProjectIDs: []string{projA}})
	if err != nil {
		t.Fatalf("CreateScope: %v", err)
	}

	for name, call := range map[string]func() error{
		"add another tenant's project":       func() error { return store.AddProjectToScope(ctxA, scopeA.ID, projB) },
		"add to another tenant's scope":      func() error { return store.AddProjectToScope(ctxA, scopeB.ID, projA) },
		"add in another tenant's name":       func() error { return store.AddProjectToScope(ctxB, scopeA.ID, projA2) },
		"add an unknown project":             func() error { return store.AddProjectToScope(ctxA, scopeA.ID, uuid.NewString()) },
		"remove from another tenant's scope": func() error { return store.RemoveProjectFromScope(ctxA, scopeB.ID, projB) },
		"replace another tenant's scope's list": func() error {
			_, err := store.UpdateScope(ctxA, scopeB.ID, cfcontext.UpdateScopeRequest{ProjectIDs: []string{projA}})
			return err
		},
		"replace with another tenant's project": func() error {
			_, err := store.UpdateScope(ctxA, scopeA.ID, cfcontext.UpdateScopeRequest{ProjectIDs: []string{projB}})
			return err
		},
		"rename another tenant's scope": func() error {
			_, err := store.UpdateScope(ctxA, scopeB.ID, cfcontext.UpdateScopeRequest{Description: new(string)})
			return err
		},
	} {
		if err := call(); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("%s = %v, want domain.ErrNotFound", name, err)
		}
	}
	if got, err := store.GetScope(ctxB, scopeB.ID); err != nil || !slices.Equal(got.ProjectIDs, []string{projB}) {
		t.Fatalf("scope B = %+v, %v; want it untouched with project B", got, err)
	}

	for range 2 { // adding a project of the scope changes nothing
		if err := store.AddProjectToScope(ctxA, scopeA.ID, projA2); err != nil {
			t.Fatalf("AddProjectToScope: %v", err)
		}
	}
	if _, err := store.UpdateScope(ctxA, scopeA.ID, cfcontext.UpdateScopeRequest{ProjectIDs: []string{projA2, projA}}); err != nil {
		t.Fatalf("UpdateScope: %v", err)
	}
	var linkTenant string
	if err := pool.QueryRow(context.Background(),
		`SELECT tenant_id::text FROM retrieval_scope_projects WHERE scope_id = $1 AND project_id = $2`, scopeA.ID, projA,
	).Scan(&linkTenant); err != nil {
		t.Fatalf("read link: %v", err)
	}
	if want := scopeTenantOf(t, pool, scopeA.ID); linkTenant != want {
		t.Errorf("link tenant = %s, want the scope's %s", linkTenant, want)
	}

	// A cross-tenant link written before the check is never listed.
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO retrieval_scope_projects (scope_id, project_id) VALUES ($1, $2)`, scopeA.ID, projB); err != nil {
		t.Fatalf("insert legacy link: %v", err)
	}
	got, err := store.GetScope(ctxA, scopeA.ID)
	if err != nil || slices.Contains(got.ProjectIDs, projB) || len(got.ProjectIDs) != 2 {
		t.Fatalf("GetScope(A) = %+v, %v; want projects A and A2 only", got, err)
	}
	if err := store.RemoveProjectFromScope(ctxA, scopeA.ID, projA2); err != nil {
		t.Errorf("RemoveProjectFromScope: %v", err)
	}
}

func scopeTenantOf(t *testing.T, pool *pgxpool.Pool, scopeID string) string {
	t.Helper()
	var tid string
	if err := pool.QueryRow(context.Background(), `SELECT tenant_id::text FROM retrieval_scopes WHERE id = $1`, scopeID).Scan(&tid); err != nil {
		t.Fatalf("scope tenant: %v", err)
	}
	return tid
}
