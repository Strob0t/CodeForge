package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/knowledgebase"
	"github.com/Strob0t/CodeForge/internal/service"
)

// kbMockStore provides a minimal mock for knowledge base store operations.
type kbMockStore struct {
	runtimeMockStore
	kbs      []knowledgebase.KnowledgeBase
	scopeKBs map[string][]string // scopeID -> []kbID
}

func newKBMockStore() *kbMockStore {
	return &kbMockStore{
		scopeKBs: make(map[string][]string),
	}
}

func (m *kbMockStore) CreateKnowledgeBase(_ context.Context, req *knowledgebase.CreateRequest) (*knowledgebase.KnowledgeBase, error) {
	kb := &knowledgebase.KnowledgeBase{
		ID:          "kb-" + req.Name,
		Name:        req.Name,
		Description: req.Description,
		Category:    req.Category,
		Tags:        req.Tags,
		ContentPath: req.ContentPath,
		Status:      "pending",
	}
	m.kbs = append(m.kbs, *kb)
	return kb, nil
}

func (m *kbMockStore) GetKnowledgeBase(_ context.Context, id string) (*knowledgebase.KnowledgeBase, error) {
	for i := range m.kbs {
		if m.kbs[i].ID == id {
			return &m.kbs[i], nil
		}
	}
	return nil, errors.New("not found")
}

func (m *kbMockStore) ListKnowledgeBases(_ context.Context) ([]knowledgebase.KnowledgeBase, error) {
	return m.kbs, nil
}

func (m *kbMockStore) UpdateKnowledgeBase(_ context.Context, id string, req knowledgebase.UpdateRequest) (*knowledgebase.KnowledgeBase, error) {
	for i := range m.kbs {
		if m.kbs[i].ID == id {
			if req.Name != nil {
				m.kbs[i].Name = *req.Name
			}
			if req.Description != nil {
				m.kbs[i].Description = *req.Description
			}
			return &m.kbs[i], nil
		}
	}
	return nil, errors.New("not found")
}

func (m *kbMockStore) DeleteKnowledgeBase(_ context.Context, id string) error {
	for i := range m.kbs {
		if m.kbs[i].ID == id {
			m.kbs = append(m.kbs[:i], m.kbs[i+1:]...)
			return nil
		}
	}
	return errors.New("not found")
}

func (m *kbMockStore) UpdateKnowledgeBaseStatus(_ context.Context, id, status string, chunkCount int) error {
	for i := range m.kbs {
		if m.kbs[i].ID == id {
			m.kbs[i].Status = knowledgebase.Status(status)
			m.kbs[i].ChunkCount = chunkCount
			return nil
		}
	}
	return errors.New("not found")
}

func (m *kbMockStore) AddKnowledgeBaseToScope(_ context.Context, scopeID, kbID string) error {
	m.scopeKBs[scopeID] = append(m.scopeKBs[scopeID], kbID)
	return nil
}

func (m *kbMockStore) RemoveKnowledgeBaseFromScope(_ context.Context, scopeID, kbID string) error {
	ids := m.scopeKBs[scopeID]
	for i, id := range ids {
		if id == kbID {
			m.scopeKBs[scopeID] = append(ids[:i], ids[i+1:]...)
			return nil
		}
	}
	return errors.New("not found")
}

func (m *kbMockStore) ListKnowledgeBasesByScope(_ context.Context, scopeID string) ([]knowledgebase.KnowledgeBase, error) {
	kbIDs := m.scopeKBs[scopeID]
	var result []knowledgebase.KnowledgeBase
	for _, kbID := range kbIDs {
		for i := range m.kbs {
			if m.kbs[i].ID == kbID {
				result = append(result, m.kbs[i])
			}
		}
	}
	return result, nil
}

func TestKnowledgeBaseService_CreateGetList(t *testing.T) {
	store := newKBMockStore()
	svc := service.NewKnowledgeBaseService(store)

	ctx := context.Background()

	// Create
	kb, err := svc.Create(ctx, &knowledgebase.CreateRequest{
		Name:        "test-kb",
		Description: "A test knowledge base",
		Category:    "framework",
		Tags:        []string{"go", "testing"},
		ContentPath: "/data/test",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if kb.Name != "test-kb" {
		t.Errorf("expected name 'test-kb', got %q", kb.Name)
	}

	// Get
	got, err := svc.Get(ctx, kb.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != kb.ID {
		t.Errorf("expected ID %q, got %q", kb.ID, got.ID)
	}

	// List
	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("expected 1 kb, got %d", len(list))
	}
}

func TestKnowledgeBaseService_DeleteSucceeds(t *testing.T) {
	store := newKBMockStore()
	svc := service.NewKnowledgeBaseService(store)
	ctx := context.Background()

	kb, _ := svc.Create(ctx, &knowledgebase.CreateRequest{
		Name:     "deleteme",
		Category: "custom",
	})

	if err := svc.Delete(ctx, kb.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	list, _ := svc.List(ctx)
	if len(list) != 0 {
		t.Errorf("expected 0 kbs after delete, got %d", len(list))
	}
}

func TestKnowledgeBaseService_ScopeAttachDetach(t *testing.T) {
	store := newKBMockStore()
	svc := service.NewKnowledgeBaseService(store)
	ctx := context.Background()

	kb, _ := svc.Create(ctx, &knowledgebase.CreateRequest{
		Name:     "scope-kb",
		Category: "language",
	})

	// Attach
	if err := svc.AttachToScope(ctx, "scope-1", kb.ID); err != nil {
		t.Fatalf("AttachToScope: %v", err)
	}

	// List by scope
	kbs, err := svc.ListByScope(ctx, "scope-1")
	if err != nil {
		t.Fatalf("ListByScope: %v", err)
	}
	if len(kbs) != 1 {
		t.Errorf("expected 1 KB in scope, got %d", len(kbs))
	}

	// Detach
	if err := svc.DetachFromScope(ctx, "scope-1", kb.ID); err != nil {
		t.Fatalf("DetachFromScope: %v", err)
	}

	kbs, _ = svc.ListByScope(ctx, "scope-1")
	if len(kbs) != 0 {
		t.Errorf("expected 0 KBs in scope after detach, got %d", len(kbs))
	}
}

func TestKnowledgeBaseService_CreateValidationError(t *testing.T) {
	store := newKBMockStore()
	svc := service.NewKnowledgeBaseService(store)
	ctx := context.Background()

	_, err := svc.Create(ctx, &knowledgebase.CreateRequest{
		Name:     "",
		Category: "framework",
	})
	if err == nil {
		t.Fatal("expected validation error for empty name, got nil")
	}

	_, err = svc.Create(ctx, &knowledgebase.CreateRequest{
		Name:     "test",
		Category: "invalid-category",
	})
	if err == nil {
		t.Fatal("expected validation error for invalid category, got nil")
	}
}
