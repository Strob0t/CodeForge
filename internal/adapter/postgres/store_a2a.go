package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Strob0t/CodeForge/internal/domain"
	a2adomain "github.com/Strob0t/CodeForge/internal/domain/a2a"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

// --- A2A Tasks ---

func (s *Store) CreateA2ATask(ctx context.Context, t *a2adomain.A2ATask) error {
	metaJSON, err := marshalJSON(t.Metadata, "metadata")
	if err != nil {
		return fmt.Errorf("create a2a task: %w", err)
	}
	now := time.Now().UTC()
	_, err = s.pool.Exec(ctx, `
		INSERT INTO a2a_tasks (id, context_id, state, direction, skill_id,
			trust_origin, trust_level, source_addr, project_id, remote_agent_id,
			tenant_id, metadata, history, artifacts, error_message, version,
			created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`,
		t.ID, t.ContextID, string(t.State), string(t.Direction), t.SkillID,
		t.TrustOrigin, t.TrustLevel, t.SourceAddr, t.ProjectID, t.RemoteAgentID,
		tenantFromCtx(ctx), metaJSON, t.History, t.Artifacts, t.ErrorMessage,
		t.Version, now, now,
	)
	if err != nil {
		return fmt.Errorf("create a2a task: %w", err)
	}
	t.CreatedAt = now
	t.UpdatedAt = now
	return nil
}

func (s *Store) GetA2ATask(ctx context.Context, id string) (*a2adomain.A2ATask, error) {
	var t a2adomain.A2ATask
	var metaJSON []byte
	var state, direction string
	err := s.pool.QueryRow(ctx, `
		SELECT id, context_id, state, direction, skill_id,
			trust_origin, trust_level, source_addr, project_id, remote_agent_id,
			tenant_id, metadata, history, artifacts, error_message, version,
			created_at, updated_at
		FROM a2a_tasks WHERE id=$1 AND tenant_id=$2`, id, tenantFromCtx(ctx)).Scan(
		&t.ID, &t.ContextID, &state, &direction, &t.SkillID,
		&t.TrustOrigin, &t.TrustLevel, &t.SourceAddr, &t.ProjectID, &t.RemoteAgentID,
		&t.TenantID, &metaJSON, &t.History, &t.Artifacts, &t.ErrorMessage, &t.Version,
		&t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return nil, notFoundWrap(err, "a2a task %s", id)
	}
	t.State = a2adomain.TaskState(state)
	t.Direction = a2adomain.Direction(direction)
	if err := unmarshalJSONField(metaJSON, &t.Metadata, "metadata"); err != nil {
		return nil, fmt.Errorf("a2a task %s: %w", id, err)
	}
	return &t, nil
}

func (s *Store) UpdateA2ATask(ctx context.Context, t *a2adomain.A2ATask) error {
	metaJSON, err := marshalJSON(t.Metadata, "metadata")
	if err != nil {
		return fmt.Errorf("update a2a task: %w", err)
	}
	now := time.Now().UTC()
	tag, err := s.pool.Exec(ctx, `
		UPDATE a2a_tasks SET
			context_id=$2, state=$3, direction=$4, skill_id=$5,
			trust_origin=$6, trust_level=$7, source_addr=$8, project_id=$9,
			remote_agent_id=$10, metadata=$11, history=$12, artifacts=$13,
			error_message=$14, version=version+1, updated_at=$15
		WHERE id=$1 AND version=$16 AND tenant_id=$17`,
		t.ID, t.ContextID, string(t.State), string(t.Direction), t.SkillID,
		t.TrustOrigin, t.TrustLevel, t.SourceAddr, t.ProjectID,
		t.RemoteAgentID, metaJSON, t.History, t.Artifacts,
		t.ErrorMessage, now, t.Version, tenantFromCtx(ctx),
	)
	if err != nil {
		return fmt.Errorf("update a2a task %s: %w", t.ID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("update a2a task %s: %w", t.ID, domain.ErrConflict)
	}
	t.Version++
	t.UpdatedAt = now
	return nil
}

func (s *Store) ListA2ATasks(ctx context.Context, filter *database.A2ATaskFilter) ([]a2adomain.A2ATask, int, error) {
	// Always enforce tenant isolation from context.
	conditions := []string{"tenant_id=$1"}
	args := []any{tenantFromCtx(ctx)}
	idx := 2

	if filter != nil {
		if filter.State != "" {
			conditions = append(conditions, fmt.Sprintf("state=$%d", idx))
			args = append(args, filter.State)
			idx++
		}
		if filter.Direction != "" {
			conditions = append(conditions, fmt.Sprintf("direction=$%d", idx))
			args = append(args, filter.Direction)
			idx++
		}
		if filter.ProjectID != "" {
			conditions = append(conditions, fmt.Sprintf("project_id=$%d", idx))
			args = append(args, filter.ProjectID)
			idx++
		}
	}

	where := "WHERE " + strings.Join(conditions, " AND ")

	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	query := fmt.Sprintf("SELECT id,context_id,state,direction,skill_id,trust_origin,trust_level,source_addr,project_id,remote_agent_id,tenant_id,metadata,history,artifacts,error_message,version,created_at,updated_at FROM a2a_tasks %s ORDER BY created_at DESC LIMIT $%d", where, idx)
	args = append(args, limit)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list a2a tasks: %w", err)
	}
	tasks, err := scanRows(rows, func(r pgx.Rows) (a2adomain.A2ATask, error) {
		var t a2adomain.A2ATask
		var metaJSON []byte
		var state, direction string
		if err := r.Scan(
			&t.ID, &t.ContextID, &state, &direction, &t.SkillID,
			&t.TrustOrigin, &t.TrustLevel, &t.SourceAddr, &t.ProjectID, &t.RemoteAgentID,
			&t.TenantID, &metaJSON, &t.History, &t.Artifacts, &t.ErrorMessage, &t.Version,
			&t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return t, err
		}
		t.State = a2adomain.TaskState(state)
		t.Direction = a2adomain.Direction(direction)
		_ = unmarshalJSONField(metaJSON, &t.Metadata, "metadata")
		return t, nil
	})
	if err != nil {
		return nil, 0, err
	}
	return tasks, len(tasks), nil
}

func (s *Store) DeleteA2ATask(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, "DELETE FROM a2a_tasks WHERE id=$1 AND tenant_id=$2", id, tenantFromCtx(ctx))
	return execExpectOne(tag, err, "delete a2a task %s", id)
}

// --- A2A Remote Agents ---

func (s *Store) CreateRemoteAgent(ctx context.Context, a *a2adomain.RemoteAgent) error {
	now := time.Now().UTC()
	err := s.pool.QueryRow(ctx, `
		INSERT INTO a2a_remote_agents (name, url, description, trust_level, enabled,
			skills, card_json, tenant_id, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id`,
		a.Name, a.URL, a.Description, a.TrustLevel, a.Enabled,
		pgTextArray(a.Skills), a.CardJSON, tenantFromCtx(ctx), now, now,
	).Scan(&a.ID)
	if err != nil {
		return fmt.Errorf("create remote agent: %w", err)
	}
	a.CreatedAt = now
	a.UpdatedAt = now
	return nil
}

func (s *Store) GetRemoteAgent(ctx context.Context, id string) (*a2adomain.RemoteAgent, error) {
	var a a2adomain.RemoteAgent
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, url, description, trust_level, enabled,
			skills, last_seen, card_json, tenant_id, created_at, updated_at
		FROM a2a_remote_agents WHERE id=$1 AND tenant_id=$2`, id, tenantFromCtx(ctx)).Scan(
		&a.ID, &a.Name, &a.URL, &a.Description, &a.TrustLevel, &a.Enabled,
		&a.Skills, &a.LastSeen, &a.CardJSON, &a.TenantID, &a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		return nil, notFoundWrap(err, "remote agent %s", id)
	}
	return &a, nil
}

func (s *Store) ListRemoteAgents(ctx context.Context, _ string, enabledOnly bool) ([]a2adomain.RemoteAgent, error) {
	// Always enforce tenant from context.
	query := "SELECT id, name, url, description, trust_level, enabled, skills, last_seen, card_json, tenant_id, created_at, updated_at FROM a2a_remote_agents WHERE tenant_id=$1"
	args := []any{tenantFromCtx(ctx)}
	idx := 2
	if enabledOnly {
		query += fmt.Sprintf(" AND enabled=$%d", idx)
		args = append(args, true)
	}
	query += " ORDER BY created_at DESC"

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list remote agents: %w", err)
	}
	return scanRows(rows, func(r pgx.Rows) (a2adomain.RemoteAgent, error) {
		var a a2adomain.RemoteAgent
		err := r.Scan(
			&a.ID, &a.Name, &a.URL, &a.Description, &a.TrustLevel, &a.Enabled,
			&a.Skills, &a.LastSeen, &a.CardJSON, &a.TenantID, &a.CreatedAt, &a.UpdatedAt,
		)
		return a, err
	})
}

func (s *Store) UpdateRemoteAgent(ctx context.Context, a *a2adomain.RemoteAgent) error {
	now := time.Now().UTC()
	tag, err := s.pool.Exec(ctx, `
		UPDATE a2a_remote_agents SET
			name=$2, url=$3, description=$4, trust_level=$5, enabled=$6,
			skills=$7, last_seen=$8, card_json=$9, updated_at=$10
		WHERE id=$1 AND tenant_id=$11`,
		a.ID, a.Name, a.URL, a.Description, a.TrustLevel, a.Enabled,
		pgTextArray(a.Skills), a.LastSeen, a.CardJSON, now, tenantFromCtx(ctx),
	)
	if err != nil {
		return execExpectOne(tag, err, "update remote agent %s", a.ID)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("update remote agent %s: %w", a.ID, domain.ErrNotFound)
	}
	a.UpdatedAt = now
	return nil
}

func (s *Store) DeleteRemoteAgent(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, "DELETE FROM a2a_remote_agents WHERE id=$1 AND tenant_id=$2", id, tenantFromCtx(ctx))
	return execExpectOne(tag, err, "delete remote agent %s", id)
}

// --- A2A Push Configs ---

func (s *Store) CreateA2APushConfig(ctx context.Context, taskID, url, token string) (string, error) {
	var id string
	// Use INSERT...FROM subquery to verify the task belongs to the calling tenant
	// before inserting the push config. Returns no rows if the task does not exist
	// or belongs to a different tenant.
	err := s.pool.QueryRow(ctx, `
		INSERT INTO a2a_push_configs (task_id, url, token, created_at)
		SELECT $1, $2, $3, $4
		FROM a2a_tasks WHERE id = $1 AND tenant_id = $5
		RETURNING id`,
		taskID, url, token, time.Now().UTC(), tenantFromCtx(ctx),
	).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", fmt.Errorf("create push config: task %s not found: %w", taskID, domain.ErrNotFound)
		}
		return "", fmt.Errorf("create push config: %w", err)
	}
	return id, nil
}

func (s *Store) GetA2APushConfig(ctx context.Context, id string) (taskID, url, token string, err error) {
	err = s.pool.QueryRow(ctx, `SELECT pc.task_id, pc.url, pc.token FROM a2a_push_configs pc JOIN a2a_tasks t ON t.id = pc.task_id WHERE pc.id = $1 AND t.tenant_id = $2`, id, tenantFromCtx(ctx)).
		Scan(&taskID, &url, &token)
	if err != nil {
		return "", "", "", notFoundWrap(err, "push config %s", id)
	}
	return taskID, url, token, nil
}

func (s *Store) ListA2APushConfigs(ctx context.Context, taskID string) ([]database.A2APushConfig, error) {
	rows, err := s.pool.Query(ctx, `SELECT pc.id, pc.task_id, pc.url, pc.token, pc.created_at FROM a2a_push_configs pc JOIN a2a_tasks t ON t.id = pc.task_id WHERE pc.task_id = $1 AND t.tenant_id = $2`, taskID, tenantFromCtx(ctx))
	if err != nil {
		return nil, fmt.Errorf("list push configs: %w", err)
	}
	return scanRows(rows, func(r pgx.Rows) (database.A2APushConfig, error) {
		var c database.A2APushConfig
		err := r.Scan(&c.ID, &c.TaskID, &c.URL, &c.Token, &c.CreatedAt)
		return c, err
	})
}

func (s *Store) DeleteA2APushConfig(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, "DELETE FROM a2a_push_configs WHERE id = $1 AND task_id IN (SELECT id FROM a2a_tasks WHERE tenant_id = $2)", id, tenantFromCtx(ctx))
	return execExpectOne(tag, err, "delete push config %s", id)
}

func (s *Store) DeleteAllA2APushConfigs(ctx context.Context, taskID string) error {
	_, err := s.pool.Exec(ctx, "DELETE FROM a2a_push_configs WHERE task_id = $1 AND task_id IN (SELECT id FROM a2a_tasks WHERE tenant_id = $2)", taskID, tenantFromCtx(ctx))
	if err != nil {
		return fmt.Errorf("delete all push configs for task %s: %w", taskID, err)
	}
	return nil
}
