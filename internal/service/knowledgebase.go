package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/knowledgebase"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
	"github.com/Strob0t/CodeForge/internal/workspacefs"
)

// KnowledgeBaseService manages knowledge base CRUD and scope attachment.
// Content lives below the knowledge content root (KI-105).
type KnowledgeBaseService struct {
	store     database.Store
	retrieval *RetrievalService
	content   *knowledgeContent
}

// NewKnowledgeBaseService creates a KnowledgeBaseService whose content lives
// below contentRoot (knowledge.content_root).
func NewKnowledgeBaseService(store database.Store, contentRoot string) *KnowledgeBaseService {
	return &KnowledgeBaseService{store: store, content: newKnowledgeContent(contentRoot)}
}

// SetRetrieval wires the retrieval service for indexing.
func (s *KnowledgeBaseService) SetRetrieval(r *RetrievalService) { s.retrieval = r }

// Create validates and creates a new knowledge base.
func (s *KnowledgeBaseService) Create(ctx context.Context, req *knowledgebase.CreateRequest) (*knowledgebase.KnowledgeBase, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("validate knowledge base: %w", err)
	}
	rel, err := s.content.relative(tenantctx.FromContext(ctx), req.ContentPath)
	if err != nil {
		return nil, err
	}
	stored := *req
	stored.ContentPath = rel
	return s.store.CreateKnowledgeBase(ctx, &stored)
}

// Get returns a knowledge base by ID.
func (s *KnowledgeBaseService) Get(ctx context.Context, id string) (*knowledgebase.KnowledgeBase, error) {
	return s.store.GetKnowledgeBase(ctx, id)
}

// List returns all knowledge bases for the current tenant.
func (s *KnowledgeBaseService) List(ctx context.Context) ([]knowledgebase.KnowledgeBase, error) {
	return s.store.ListKnowledgeBases(ctx)
}

// Update applies partial updates to a knowledge base.
func (s *KnowledgeBaseService) Update(ctx context.Context, id string, req knowledgebase.UpdateRequest) (*knowledgebase.KnowledgeBase, error) {
	return s.store.UpdateKnowledgeBase(ctx, id, req)
}

// Delete removes a knowledge base by ID.
func (s *KnowledgeBaseService) Delete(ctx context.Context, id string) error {
	return s.store.DeleteKnowledgeBase(ctx, id)
}

// AttachToScope adds a knowledge base to a scope.
func (s *KnowledgeBaseService) AttachToScope(ctx context.Context, scopeID, kbID string) error {
	return s.store.AddKnowledgeBaseToScope(ctx, scopeID, kbID)
}

// DetachFromScope removes a knowledge base from a scope.
func (s *KnowledgeBaseService) DetachFromScope(ctx context.Context, scopeID, kbID string) error {
	return s.store.RemoveKnowledgeBaseFromScope(ctx, scopeID, kbID)
}

// ListByScope returns knowledge bases attached to a scope.
func (s *KnowledgeBaseService) ListByScope(ctx context.Context, scopeID string) ([]knowledgebase.KnowledgeBase, error) {
	return s.store.ListKnowledgeBasesByScope(ctx, scopeID)
}

// RequestIndex triggers indexing of a knowledge base's content via the retrieval pipeline.
// The knowledge base content is indexed using "kb:<id>" as the project identifier.
func (s *KnowledgeBaseService) RequestIndex(ctx context.Context, id string) error {
	if s.retrieval == nil {
		return fmt.Errorf("retrieval service not configured")
	}

	kb, err := s.store.GetKnowledgeBase(ctx, id)
	if err != nil {
		return fmt.Errorf("get knowledge base: %w", err)
	}

	if kb.ContentPath == "" {
		return fmt.Errorf("knowledge base %q has no content path: %w", kb.Name, domain.ErrValidation)
	}
	tenant := tenantctx.FromContext(ctx)
	rel, err := s.content.stored(tenant, kb)
	if err != nil {
		return err
	}
	if err := s.checkContent(tenant, rel); err != nil {
		return err
	}

	// Use "kb:<id>" as the project identifier to namespace KB indexes; the
	// worker indexes rel below the tenant's area of its own content root
	// (the payload carries the tenant).
	kbProjectID := "kb:" + kb.ID
	if err := s.retrieval.RequestKnowledgeIndex(ctx, kbProjectID, rel); err != nil {
		return fmt.Errorf("request index for knowledge base: %w", err)
	}

	if err := s.store.UpdateKnowledgeBaseStatus(ctx, id, "pending", 0); err != nil {
		slog.Warn("failed to update knowledge base status", "id", id, "error", err)
	}

	return nil
}

// checkContent verifies that rel names a directory or a regular file inside
// the tenant's knowledge area. Every refusal is the same 400, so a tenant
// cannot probe which paths exist (KI-105); the reason is logged.
func (s *KnowledgeBaseService) checkContent(tenant, rel string) error {
	root, err := s.content.open(tenant)
	if err != nil {
		slog.Info("knowledge content refused", "tenant_id", tenant, "content_path", rel, "reason", err)
		return knowledgeUnavailable()
	}
	defer func() { _ = root.Close() }()
	info, err := root.Stat(rel)
	if err == nil && !info.IsDir() && !info.Mode().IsRegular() {
		err = workspacefs.ErrNotRegular
	}
	if err != nil {
		slog.Info("knowledge content refused", "tenant_id", tenant, "content_path", rel, "reason", err)
		return knowledgeUnavailable()
	}
	return nil
}

// ReadContent returns up to maxBytes of a knowledge base of the caller's
// tenant whose content_path names a regular file in the tenant's knowledge
// area, and whether it was cut.
func (s *KnowledgeBaseService) ReadContent(ctx context.Context, kb *knowledgebase.KnowledgeBase, maxBytes int64) (data []byte, truncated bool, err error) {
	tenant := tenantctx.FromContext(ctx)
	rel, err := s.content.stored(tenant, kb)
	if err != nil {
		return nil, false, err
	}
	root, err := s.content.open(tenant)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = root.Close() }()
	data, _, truncated, err = root.ReadFilePrefix(rel, maxBytes)
	return data, truncated, err
}
