package postgres

import (
	"context"
	"fmt"

	"github.com/Strob0t/CodeForge/internal/domain/prompt"
)

// ListPromptSections returns all prompt sections for a given scope.
func (s *Store) ListPromptSections(ctx context.Context, scope string) ([]prompt.SectionRow, error) {
	tid := tenantFromCtx(ctx)

	rows, err := s.pool.Query(ctx,
		`SELECT id, name, scope, content, priority, sort_order, enabled, merge
		 FROM prompt_sections
		 WHERE tenant_id = $1 AND scope = $2
		 ORDER BY sort_order ASC, priority DESC`, tid, scope)
	if err != nil {
		return nil, fmt.Errorf("list prompt sections: %w", err)
	}
	defer rows.Close()

	var sections []prompt.SectionRow
	for rows.Next() {
		var r prompt.SectionRow
		if err := rows.Scan(&r.ID, &r.Name, &r.Scope, &r.Content, &r.Priority, &r.SortOrder, &r.Enabled, &r.Merge); err != nil {
			return nil, fmt.Errorf("scan prompt section: %w", err)
		}
		sections = append(sections, r)
	}
	return sections, rows.Err()
}

// UpsertPromptSection creates or updates a prompt section.
func (s *Store) UpsertPromptSection(ctx context.Context, row *prompt.SectionRow) error {
	tid := tenantFromCtx(ctx)

	_, err := s.pool.Exec(ctx,
		`INSERT INTO prompt_sections (tenant_id, name, scope, content, priority, sort_order, enabled, merge)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT (tenant_id, scope, name) DO UPDATE SET
		   content = EXCLUDED.content,
		   priority = EXCLUDED.priority,
		   sort_order = EXCLUDED.sort_order,
		   enabled = EXCLUDED.enabled,
		   merge = EXCLUDED.merge,
		   updated_at = now()`,
		tid, row.Name, row.Scope, row.Content, row.Priority, row.SortOrder, row.Enabled, row.Merge)
	if err != nil {
		return fmt.Errorf("upsert prompt section: %w", err)
	}
	return nil
}

// DeletePromptSection removes a prompt section by ID.
func (s *Store) DeletePromptSection(ctx context.Context, id string) error {
	tid := tenantFromCtx(ctx)
	_, err := s.pool.Exec(ctx,
		`DELETE FROM prompt_sections WHERE id = $1 AND tenant_id = $2`, id, tid)
	if err != nil {
		return fmt.Errorf("delete prompt section: %w", err)
	}
	return nil
}
