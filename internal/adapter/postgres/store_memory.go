package postgres

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/Strob0t/CodeForge/internal/domain/memory"
)

// CreateMemory inserts a new agent memory into the database.
func (s *Store) CreateMemory(ctx context.Context, m *memory.Memory) error {
	const q = `
		INSERT INTO agent_memories (tenant_id, project_id, agent_id, run_id, content, kind, importance, embedding, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at`

	metadata := []byte(`{}`)
	if m.Metadata != nil {
		b, err := marshalJSON(m.Metadata, "memory metadata")
		if err != nil {
			return err
		}
		metadata = b
	}

	return s.pool.QueryRow(ctx, q,
		tenantFromCtx(ctx), m.ProjectID, m.AgentID, m.RunID, m.Content,
		string(m.Kind), m.Importance, m.Embedding, metadata,
	).Scan(&m.ID, &m.CreatedAt)
}

// ListMemories returns all memories for a project, ordered by creation time descending.
func (s *Store) ListMemories(ctx context.Context, projectID string) ([]memory.Memory, error) {
	const q = `
		SELECT id, tenant_id, project_id, agent_id, run_id, content, kind, importance, metadata, created_at
		FROM agent_memories
		WHERE project_id = $1 AND tenant_id = $2
		ORDER BY created_at DESC
		LIMIT $3`

	rows, err := s.pool.Query(ctx, q, projectID, tenantFromCtx(ctx), DefaultListLimit)
	if err != nil {
		return nil, fmt.Errorf("list memories: %w", err)
	}
	return scanRows(rows, func(r pgx.Rows) (memory.Memory, error) {
		var m memory.Memory
		var metadata []byte
		if err := r.Scan(
			&m.ID, &m.TenantID, &m.ProjectID, &m.AgentID, &m.RunID,
			&m.Content, &m.Kind, &m.Importance, &metadata, &m.CreatedAt,
		); err != nil {
			return m, err
		}
		if err := unmarshalJSONField(metadata, &m.Metadata, "metadata"); err != nil {
			slog.Warn("failed to unmarshal memory metadata", "memory_id", m.ID, "error", err)
		}
		return m, nil
	})
}
