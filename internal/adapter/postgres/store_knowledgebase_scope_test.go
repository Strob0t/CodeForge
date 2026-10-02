package postgres_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/Strob0t/CodeForge/internal/adapter/postgres"
	"github.com/Strob0t/CodeForge/internal/domain"
	cfcontext "github.com/Strob0t/CodeForge/internal/domain/context"
	"github.com/Strob0t/CodeForge/internal/domain/knowledgebase"
)

func createTestKB(ctx context.Context, t *testing.T, store *postgres.Store) *knowledgebase.KnowledgeBase {
	t.Helper()
	kb, err := store.CreateKnowledgeBase(ctx, &knowledgebase.CreateRequest{
		Name: "kb-" + uuid.NewString()[:8], Category: knowledgebase.CategoryCustom, ContentPath: "docs",
	})
	if err != nil {
		t.Fatalf("CreateKnowledgeBase: %v", err)
	}
	return kb
}

func createTestScope(ctx context.Context, t *testing.T, store *postgres.Store) *cfcontext.RetrievalScope {
	t.Helper()
	scope, err := store.CreateScope(ctx, cfcontext.CreateScopeRequest{Name: "scope-" + uuid.NewString()[:8], Type: cfcontext.ScopeShared})
	if err != nil {
		t.Fatalf("CreateScope: %v", err)
	}
	return scope
}

func kbIDs(kbs []knowledgebase.KnowledgeBase) []string {
	ids := make([]string, 0, len(kbs))
	for i := range kbs {
		ids = append(ids, kbs[i].ID)
	}
	return ids
}

// TestStore_ScopeKnowledgeBasesStayInTheTenant (KI-105 round 3): a knowledge
// base is attached only to a scope of its own tenant, in that tenant's name;
// the listing and detaching see only the caller's tenant.
func TestStore_ScopeKnowledgeBasesStayInTheTenant(t *testing.T) {
	store := setupStore(t)
	pool := retentionPool(t)
	tenantA := createTestTenant(t, store)
	ctxA := ctxWithTenant(t, tenantA)
	ctxB := ctxWithTenant(t, createTestTenant(t, store))
	kbA, kbB := createTestKB(ctxA, t, store), createTestKB(ctxB, t, store)
	scopeA, scopeB := createTestScope(ctxA, t, store), createTestScope(ctxB, t, store)

	for name, call := range map[string]func() error{
		"another tenant's knowledge base to the own scope": func() error { return store.AddKnowledgeBaseToScope(ctxA, scopeA.ID, kbB.ID) },
		"the own knowledge base to another tenant's scope": func() error { return store.AddKnowledgeBaseToScope(ctxA, scopeB.ID, kbA.ID) },
		"in another tenant's name":                         func() error { return store.AddKnowledgeBaseToScope(ctxB, scopeA.ID, kbA.ID) },
		"an unknown knowledge base":                        func() error { return store.AddKnowledgeBaseToScope(ctxA, scopeA.ID, uuid.NewString()) },
	} {
		if err := call(); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("attach %s = %v, want domain.ErrNotFound", name, err)
		}
	}

	for range 2 { // attaching an attached knowledge base changes nothing
		if err := store.AddKnowledgeBaseToScope(ctxA, scopeA.ID, kbA.ID); err != nil {
			t.Fatalf("AddKnowledgeBaseToScope: %v", err)
		}
	}
	var linkTenant string
	if err := pool.QueryRow(context.Background(),
		`SELECT tenant_id::text FROM scope_knowledge_bases WHERE scope_id = $1 AND knowledge_base_id = $2`, scopeA.ID, kbA.ID,
	).Scan(&linkTenant); err != nil {
		t.Fatalf("read link: %v", err)
	}
	if linkTenant != tenantA {
		t.Errorf("link tenant = %s, want %s", linkTenant, tenantA)
	}

	// A cross-tenant link written before the check is never listed.
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO scope_knowledge_bases (scope_id, knowledge_base_id) VALUES ($1, $2)`, scopeA.ID, kbB.ID); err != nil {
		t.Fatalf("insert legacy link: %v", err)
	}
	if kbs, err := store.ListKnowledgeBasesByScope(ctxA, scopeA.ID); err != nil || len(kbs) != 1 || kbs[0].ID != kbA.ID {
		t.Errorf("ListKnowledgeBasesByScope(A, scope A) = %v, %v; want only kb A", kbIDs(kbs), err)
	}
	if kbs, err := store.ListKnowledgeBasesByScope(ctxB, scopeA.ID); err != nil || len(kbs) != 0 {
		t.Errorf("ListKnowledgeBasesByScope(B, scope A) = %v, %v; want nothing", kbIDs(kbs), err)
	}

	if err := store.RemoveKnowledgeBaseFromScope(ctxB, scopeA.ID, kbA.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("detach in another tenant's name = %v, want domain.ErrNotFound", err)
	}
	if err := store.RemoveKnowledgeBaseFromScope(ctxA, scopeA.ID, kbA.ID); err != nil {
		t.Errorf("RemoveKnowledgeBaseFromScope: %v", err)
	}
}
