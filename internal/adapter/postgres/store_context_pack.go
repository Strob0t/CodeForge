package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	cfcontext "github.com/Strob0t/CodeForge/internal/domain/context"
)

// --- Context Packs ---

// CreateContextPack inserts a context pack and its entries in a transaction.
func (s *Store) CreateContextPack(ctx context.Context, pack *cfcontext.ContextPack) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback after commit is a no-op

	tid := tenantFromCtx(ctx)

	err = tx.QueryRow(ctx,
		`INSERT INTO context_packs (tenant_id, task_id, project_id, token_budget, tokens_used)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id, created_at`,
		tid, pack.TaskID, pack.ProjectID, pack.TokenBudget, pack.TokensUsed,
	).Scan(&pack.ID, &pack.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert context_pack: %w", err)
	}

	for i := range pack.Entries {
		e := &pack.Entries[i]
		e.PackID = pack.ID
		err = tx.QueryRow(ctx,
			`INSERT INTO context_entries (tenant_id, pack_id, kind, path, content, tokens, priority)
			 VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
			tid, e.PackID, e.Kind, e.Path, e.Content, e.Tokens, e.Priority,
		).Scan(&e.ID)
		if err != nil {
			return fmt.Errorf("insert context_entry %d: %w", i, err)
		}
	}

	return tx.Commit(ctx)
}

// GetContextPack returns a context pack by ID with all entries.
func (s *Store) GetContextPack(ctx context.Context, id string) (*cfcontext.ContextPack, error) {
	var p cfcontext.ContextPack
	err := s.pool.QueryRow(ctx,
		`SELECT id, task_id, project_id, token_budget, tokens_used, created_at
		 FROM context_packs WHERE id = $1 AND tenant_id = $2`, id, tenantFromCtx(ctx),
	).Scan(&p.ID, &p.TaskID, &p.ProjectID, &p.TokenBudget, &p.TokensUsed, &p.CreatedAt)
	if err != nil {
		return nil, notFoundWrap(err, "get context_pack %s", id)
	}

	entries, err := s.loadContextEntries(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	p.Entries = entries
	return &p, nil
}

// GetContextPackByTask returns the context pack for a task.
func (s *Store) GetContextPackByTask(ctx context.Context, taskID string) (*cfcontext.ContextPack, error) {
	var p cfcontext.ContextPack
	err := s.pool.QueryRow(ctx,
		`SELECT id, task_id, project_id, token_budget, tokens_used, created_at
		 FROM context_packs WHERE task_id = $1 AND tenant_id = $2 ORDER BY created_at DESC LIMIT 1`, taskID, tenantFromCtx(ctx),
	).Scan(&p.ID, &p.TaskID, &p.ProjectID, &p.TokenBudget, &p.TokensUsed, &p.CreatedAt)
	if err != nil {
		return nil, notFoundWrap(err, "get context_pack by task %s", taskID)
	}

	entries, err := s.loadContextEntries(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	p.Entries = entries
	return &p, nil
}

// DeleteContextPack removes a context pack and its entries (CASCADE).
func (s *Store) DeleteContextPack(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM context_packs WHERE id = $1 AND tenant_id = $2`, id, tenantFromCtx(ctx))
	return execExpectOne(tag, err, "delete context_pack %s", id)
}

func (s *Store) loadContextEntries(ctx context.Context, packID string) ([]cfcontext.ContextEntry, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, pack_id, kind, path, content, tokens, priority
		 FROM context_entries WHERE pack_id = $1 AND tenant_id = $2 ORDER BY priority DESC`, packID, tenantFromCtx(ctx))
	if err != nil {
		return nil, fmt.Errorf("load context_entries: %w", err)
	}
	return scanRows(rows, func(r pgx.Rows) (cfcontext.ContextEntry, error) {
		var e cfcontext.ContextEntry
		err := r.Scan(&e.ID, &e.PackID, &e.Kind, &e.Path, &e.Content, &e.Tokens, &e.Priority)
		return e, err
	})
}

// --- Shared Context ---

// CreateSharedContext inserts a new shared context for a team.
func (s *Store) CreateSharedContext(ctx context.Context, sc *cfcontext.SharedContext) error {
	return s.pool.QueryRow(ctx,
		`INSERT INTO shared_contexts (tenant_id, team_id, project_id) VALUES ($1, $2, $3)
		 RETURNING id, version, created_at, updated_at`,
		tenantFromCtx(ctx), sc.TeamID, sc.ProjectID,
	).Scan(&sc.ID, &sc.Version, &sc.CreatedAt, &sc.UpdatedAt)
}

// GetSharedContext returns a shared context by ID with all items.
func (s *Store) GetSharedContext(ctx context.Context, id string) (*cfcontext.SharedContext, error) {
	var sc cfcontext.SharedContext
	err := s.pool.QueryRow(ctx,
		`SELECT id, team_id, project_id, version, created_at, updated_at
		 FROM shared_contexts WHERE id = $1 AND tenant_id = $2`, id, tenantFromCtx(ctx),
	).Scan(&sc.ID, &sc.TeamID, &sc.ProjectID, &sc.Version, &sc.CreatedAt, &sc.UpdatedAt)
	if err != nil {
		return nil, notFoundWrap(err, "get shared_context %s", id)
	}

	items, err := s.loadSharedContextItems(ctx, sc.ID)
	if err != nil {
		return nil, err
	}
	sc.Items = items
	return &sc, nil
}

// GetSharedContextByTeam returns the shared context for a team.
func (s *Store) GetSharedContextByTeam(ctx context.Context, teamID string) (*cfcontext.SharedContext, error) {
	var sc cfcontext.SharedContext
	err := s.pool.QueryRow(ctx,
		`SELECT id, team_id, project_id, version, created_at, updated_at
		 FROM shared_contexts WHERE team_id = $1 AND tenant_id = $2`, teamID, tenantFromCtx(ctx),
	).Scan(&sc.ID, &sc.TeamID, &sc.ProjectID, &sc.Version, &sc.CreatedAt, &sc.UpdatedAt)
	if err != nil {
		return nil, notFoundWrap(err, "get shared_context by team %s", teamID)
	}

	items, err := s.loadSharedContextItems(ctx, sc.ID)
	if err != nil {
		return nil, err
	}
	sc.Items = items
	return &sc, nil
}

// AddSharedContextItem inserts a new item and bumps the shared context version.
func (s *Store) AddSharedContextItem(ctx context.Context, req cfcontext.AddSharedItemRequest) (*cfcontext.SharedContextItem, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback after commit is a no-op

	tid := tenantFromCtx(ctx)

	// Resolve shared context ID from team ID.
	var sharedID string
	err = tx.QueryRow(ctx,
		`SELECT id FROM shared_contexts WHERE team_id = $1 AND tenant_id = $2`, req.TeamID, tid,
	).Scan(&sharedID)
	if err != nil {
		return nil, notFoundWrap(err, "resolve shared_context for team %s", req.TeamID)
	}

	tokens := cfcontext.EstimateTokens(req.Value)
	var item cfcontext.SharedContextItem
	err = tx.QueryRow(ctx,
		`INSERT INTO shared_context_items (tenant_id, shared_id, key, value, author, tokens)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (shared_id, key) DO UPDATE SET value = EXCLUDED.value, author = EXCLUDED.author, tokens = EXCLUDED.tokens
		 RETURNING id, shared_id, key, value, author, tokens, created_at`,
		tid, sharedID, req.Key, req.Value, req.Author, tokens,
	).Scan(&item.ID, &item.SharedID, &item.Key, &item.Value, &item.Author, &item.Tokens, &item.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("upsert shared_context_item: %w", err)
	}

	// Bump version.
	if _, err := tx.Exec(ctx,
		`UPDATE shared_contexts SET version = version + 1 WHERE id = $1 AND tenant_id = $2`, sharedID, tid,
	); err != nil {
		return nil, fmt.Errorf("bump shared_context version: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return &item, nil
}

// DeleteSharedContext removes a shared context and its items (CASCADE).
func (s *Store) DeleteSharedContext(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM shared_contexts WHERE id = $1 AND tenant_id = $2`, id, tenantFromCtx(ctx))
	return execExpectOne(tag, err, "delete shared_context %s", id)
}

// --- Repo Maps ---

// UpsertRepoMap inserts or updates a repo map for a project.
func (s *Store) UpsertRepoMap(ctx context.Context, m *cfcontext.RepoMap) error {
	err := s.pool.QueryRow(ctx,
		`INSERT INTO repo_maps (tenant_id, project_id, map_text, token_count, file_count, symbol_count, languages, version)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, 1)
		 ON CONFLICT (project_id) DO UPDATE SET
		   map_text = EXCLUDED.map_text,
		   token_count = EXCLUDED.token_count,
		   file_count = EXCLUDED.file_count,
		   symbol_count = EXCLUDED.symbol_count,
		   languages = EXCLUDED.languages,
		   version = repo_maps.version + 1,
		   updated_at = now()
		 RETURNING id, version, created_at, updated_at`,
		tenantFromCtx(ctx), m.ProjectID, m.MapText, m.TokenCount, m.FileCount, m.SymbolCount, m.Languages,
	).Scan(&m.ID, &m.Version, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		return fmt.Errorf("upsert repo_map: %w", err)
	}
	return nil
}

// GetRepoMap returns the repo map for a project.
func (s *Store) GetRepoMap(ctx context.Context, projectID string) (*cfcontext.RepoMap, error) {
	var m cfcontext.RepoMap
	err := s.pool.QueryRow(ctx,
		`SELECT id, project_id, map_text, token_count, file_count, symbol_count, languages, version, created_at, updated_at
		 FROM repo_maps WHERE project_id = $1 AND tenant_id = $2`, projectID, tenantFromCtx(ctx),
	).Scan(&m.ID, &m.ProjectID, &m.MapText, &m.TokenCount, &m.FileCount, &m.SymbolCount, &m.Languages, &m.Version, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		return nil, notFoundWrap(err, "get repo_map for project %s", projectID)
	}
	return &m, nil
}

// DeleteRepoMap removes the repo map for a project.
func (s *Store) DeleteRepoMap(ctx context.Context, projectID string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM repo_maps WHERE project_id = $1 AND tenant_id = $2`, projectID, tenantFromCtx(ctx))
	return execExpectOne(tag, err, "delete repo_map for project %s", projectID)
}

func (s *Store) loadSharedContextItems(ctx context.Context, sharedID string) ([]cfcontext.SharedContextItem, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, shared_id, key, value, author, tokens, created_at
		 FROM shared_context_items WHERE shared_id = $1 AND tenant_id = $2 ORDER BY created_at`, sharedID, tenantFromCtx(ctx))
	if err != nil {
		return nil, fmt.Errorf("load shared_context_items: %w", err)
	}
	return scanRows(rows, func(r pgx.Rows) (cfcontext.SharedContextItem, error) {
		var item cfcontext.SharedContextItem
		err := r.Scan(&item.ID, &item.SharedID, &item.Key, &item.Value, &item.Author, &item.Tokens, &item.CreatedAt)
		return item, err
	})
}
