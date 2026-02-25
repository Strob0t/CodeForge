package service

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/knowledgebase"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

// KnowledgeBaseService manages knowledge base CRUD and scope attachment.
type KnowledgeBaseService struct {
	store     database.Store
	retrieval *RetrievalService
}

// NewKnowledgeBaseService creates a KnowledgeBaseService.
func NewKnowledgeBaseService(store database.Store) *KnowledgeBaseService {
	return &KnowledgeBaseService{store: store}
}

// SetRetrieval wires the retrieval service for indexing.
func (s *KnowledgeBaseService) SetRetrieval(r *RetrievalService) { s.retrieval = r }

// Create validates and creates a new knowledge base.
func (s *KnowledgeBaseService) Create(ctx context.Context, req *knowledgebase.CreateRequest) (*knowledgebase.KnowledgeBase, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("validate knowledge base: %w", err)
	}
	return s.store.CreateKnowledgeBase(ctx, req)
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

	// Validate that content_path is absolute and exists on disk.
	if !filepath.IsAbs(kb.ContentPath) {
		return fmt.Errorf("content_path must be an absolute path: %w", domain.ErrValidation)
	}
	if _, err := os.Stat(kb.ContentPath); err != nil {
		return fmt.Errorf("content_path %q does not exist: %w", kb.ContentPath, domain.ErrValidation)
	}

	// Use "kb:<id>" as the project identifier to namespace KB indexes.
	kbProjectID := "kb:" + kb.ID
	if err := s.retrieval.RequestIndex(ctx, kbProjectID, kb.ContentPath, ""); err != nil {
		return fmt.Errorf("request index for knowledge base: %w", err)
	}

	if err := s.store.UpdateKnowledgeBaseStatus(ctx, id, "pending", 0); err != nil {
		slog.Warn("failed to update knowledge base status", "id", id, "error", err)
	}

	return nil
}
