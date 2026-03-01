package postgres

import (
	"context"
	"fmt"

	"github.com/Strob0t/CodeForge/internal/domain/knowledgebase"
)

// --- Knowledge Base CRUD ---

func (s *Store) CreateKnowledgeBase(ctx context.Context, req *knowledgebase.CreateRequest) (*knowledgebase.KnowledgeBase, error) {
	tid := tenantFromCtx(ctx)

	tags := orEmpty(req.Tags)

	var kb knowledgebase.KnowledgeBase
	err := s.pool.QueryRow(ctx,
		`INSERT INTO knowledge_bases (tenant_id, name, description, category, tags, content_path)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, name, description, category, tags, content_path, status, chunk_count, created_at, updated_at`,
		tid, req.Name, req.Description, string(req.Category), tags, req.ContentPath,
	).Scan(&kb.ID, &kb.Name, &kb.Description, &kb.Category, &kb.Tags, &kb.ContentPath, &kb.Status, &kb.ChunkCount, &kb.CreatedAt, &kb.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert knowledge base: %w", err)
	}

	return &kb, nil
}

func (s *Store) GetKnowledgeBase(ctx context.Context, id string) (*knowledgebase.KnowledgeBase, error) {
	tid := tenantFromCtx(ctx)

	var kb knowledgebase.KnowledgeBase
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, description, category, tags, content_path, status, chunk_count, created_at, updated_at
		 FROM knowledge_bases WHERE id = $1 AND tenant_id = $2`, id, tid,
	).Scan(&kb.ID, &kb.Name, &kb.Description, &kb.Category, &kb.Tags, &kb.ContentPath, &kb.Status, &kb.ChunkCount, &kb.CreatedAt, &kb.UpdatedAt)
	if err != nil {
		return nil, notFoundWrap(err, "get knowledge base %s", id)
	}

	return &kb, nil
}

func (s *Store) ListKnowledgeBases(ctx context.Context) ([]knowledgebase.KnowledgeBase, error) {
	tid := tenantFromCtx(ctx)

	rows, err := s.pool.Query(ctx,
		`SELECT id, name, description, category, tags, content_path, status, chunk_count, created_at, updated_at
		 FROM knowledge_bases WHERE tenant_id = $1 ORDER BY name ASC`, tid)
	if err != nil {
		return nil, fmt.Errorf("list knowledge bases: %w", err)
	}
	defer rows.Close()

	var kbs []knowledgebase.KnowledgeBase
	for rows.Next() {
		var kb knowledgebase.KnowledgeBase
		if err := rows.Scan(&kb.ID, &kb.Name, &kb.Description, &kb.Category, &kb.Tags, &kb.ContentPath, &kb.Status, &kb.ChunkCount, &kb.CreatedAt, &kb.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan knowledge base: %w", err)
		}
		kbs = append(kbs, kb)
	}
	return kbs, rows.Err()
}

func (s *Store) UpdateKnowledgeBase(ctx context.Context, id string, req knowledgebase.UpdateRequest) (*knowledgebase.KnowledgeBase, error) {
	tid := tenantFromCtx(ctx)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if req.Name != nil {
		tag, err := tx.Exec(ctx,
			`UPDATE knowledge_bases SET name = $1, updated_at = now() WHERE id = $2 AND tenant_id = $3`,
			*req.Name, id, tid)
		if err := execExpectOne(tag, err, "update knowledge base %s", id); err != nil {
			return nil, err
		}
	}
	if req.Description != nil {
		_, err := tx.Exec(ctx,
			`UPDATE knowledge_bases SET description = $1, updated_at = now() WHERE id = $2 AND tenant_id = $3`,
			*req.Description, id, tid)
		if err != nil {
			return nil, fmt.Errorf("update knowledge base description: %w", err)
		}
	}
	if req.Tags != nil {
		_, err := tx.Exec(ctx,
			`UPDATE knowledge_bases SET tags = $1, updated_at = now() WHERE id = $2 AND tenant_id = $3`,
			req.Tags, id, tid)
		if err != nil {
			return nil, fmt.Errorf("update knowledge base tags: %w", err)
		}
	}

	if err = tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}

	return s.GetKnowledgeBase(ctx, id)
}

func (s *Store) DeleteKnowledgeBase(ctx context.Context, id string) error {
	tid := tenantFromCtx(ctx)
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM knowledge_bases WHERE id = $1 AND tenant_id = $2`, id, tid)
	return execExpectOne(tag, err, "delete knowledge base %s", id)
}

func (s *Store) UpdateKnowledgeBaseStatus(ctx context.Context, id, status string, chunkCount int) error {
	tid := tenantFromCtx(ctx)
	tag, err := s.pool.Exec(ctx,
		`UPDATE knowledge_bases SET status = $1, chunk_count = $2, updated_at = now() WHERE id = $3 AND tenant_id = $4`,
		status, chunkCount, id, tid)
	return execExpectOne(tag, err, "update knowledge base %s status", id)
}

// --- Scope ↔ Knowledge Base join table ---

func (s *Store) AddKnowledgeBaseToScope(ctx context.Context, scopeID, kbID string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO scope_knowledge_bases (scope_id, knowledge_base_id) VALUES ($1, $2)
		 ON CONFLICT DO NOTHING`,
		scopeID, kbID)
	if err != nil {
		return fmt.Errorf("add knowledge base to scope: %w", err)
	}
	return nil
}

func (s *Store) RemoveKnowledgeBaseFromScope(ctx context.Context, scopeID, kbID string) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM scope_knowledge_bases WHERE scope_id = $1 AND knowledge_base_id = $2`,
		scopeID, kbID)
	return execExpectOne(tag, err, "knowledge base %s not in scope %s", kbID, scopeID)
}

func (s *Store) ListKnowledgeBasesByScope(ctx context.Context, scopeID string) ([]knowledgebase.KnowledgeBase, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT kb.id, kb.name, kb.description, kb.category, kb.tags, kb.content_path, kb.status, kb.chunk_count, kb.created_at, kb.updated_at
		 FROM knowledge_bases kb
		 JOIN scope_knowledge_bases skb ON kb.id = skb.knowledge_base_id
		 WHERE skb.scope_id = $1
		 ORDER BY kb.name ASC`, scopeID)
	if err != nil {
		return nil, fmt.Errorf("list knowledge bases by scope: %w", err)
	}
	defer rows.Close()

	var kbs []knowledgebase.KnowledgeBase
	for rows.Next() {
		var kb knowledgebase.KnowledgeBase
		if err := rows.Scan(&kb.ID, &kb.Name, &kb.Description, &kb.Category, &kb.Tags, &kb.ContentPath, &kb.Status, &kb.ChunkCount, &kb.CreatedAt, &kb.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan knowledge base: %w", err)
		}
		kbs = append(kbs, kb)
	}
	return kbs, rows.Err()
}
