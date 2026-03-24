package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Strob0t/CodeForge/internal/domain/run"
)

// --- Sessions ---

func (s *Store) CreateSession(ctx context.Context, sess *run.Session) error {
	tid := tenantFromCtx(ctx)
	err := s.pool.QueryRow(ctx,
		`INSERT INTO sessions (tenant_id, project_id, task_id, conversation_id, parent_session_id, parent_run_id, current_run_id, status, metadata)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 RETURNING id, created_at, updated_at`,
		tid, sess.ProjectID, nullIfEmpty(sess.TaskID), nullIfEmpty(sess.ConversationID),
		nullIfEmpty(sess.ParentSessionID), nullIfEmpty(sess.ParentRunID),
		nullIfEmpty(sess.CurrentRunID), string(sess.Status), sess.Metadata,
	).Scan(&sess.ID, &sess.CreatedAt, &sess.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	sess.TenantID = tid
	return nil
}

func (s *Store) GetSession(ctx context.Context, id string) (*run.Session, error) {
	sess, err := scanSession(s.pool.QueryRow(ctx,
		`SELECT id, tenant_id, project_id, COALESCE(task_id::text, ''), COALESCE(conversation_id::text, ''),
		        COALESCE(parent_session_id::text, ''), COALESCE(parent_run_id::text, ''),
		        COALESCE(current_run_id::text, ''), status, COALESCE(metadata::text, '{}'), created_at, updated_at
		 FROM sessions WHERE id = $1 AND tenant_id = $2`, id, tenantFromCtx(ctx)))
	if err != nil {
		return nil, notFoundWrap(err, "get session %s", id)
	}
	return &sess, nil
}

func (s *Store) GetSessionByConversation(ctx context.Context, conversationID string) (*run.Session, error) {
	sess, err := scanSession(s.pool.QueryRow(ctx,
		`SELECT id, tenant_id, project_id, COALESCE(task_id::text, ''), COALESCE(conversation_id::text, ''),
		        COALESCE(parent_session_id::text, ''), COALESCE(parent_run_id::text, ''),
		        COALESCE(current_run_id::text, ''), status, COALESCE(metadata::text, '{}'), created_at, updated_at
		 FROM sessions WHERE conversation_id = $1 AND tenant_id = $2 ORDER BY created_at DESC LIMIT 1`,
		conversationID, tenantFromCtx(ctx)))
	if err != nil {
		return nil, notFoundWrap(err, "get session by conversation %s", conversationID)
	}
	return &sess, nil
}

func (s *Store) ListSessions(ctx context.Context, projectID string) ([]run.Session, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, tenant_id, project_id, COALESCE(task_id::text, ''), COALESCE(conversation_id::text, ''),
		        COALESCE(parent_session_id::text, ''), COALESCE(parent_run_id::text, ''),
		        COALESCE(current_run_id::text, ''), status, COALESCE(metadata::text, '{}'), created_at, updated_at
		 FROM sessions WHERE project_id = $1 AND tenant_id = $2 ORDER BY created_at DESC`, projectID, tenantFromCtx(ctx))
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	return scanRows(rows, func(r pgx.Rows) (run.Session, error) {
		return scanSession(r)
	})
}

func (s *Store) UpdateSessionStatus(ctx context.Context, id string, status run.SessionStatus, currentRunID string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE sessions SET status = $1, current_run_id = $2 WHERE id = $3 AND tenant_id = $4`,
		string(status), nullIfEmpty(currentRunID), id, tenantFromCtx(ctx))
	return execExpectOne(tag, err, "update session status %s", id)
}

// scanSession scans a single row into a Session.
func scanSession(row scannable) (run.Session, error) {
	var sess run.Session
	err := row.Scan(
		&sess.ID, &sess.TenantID, &sess.ProjectID, &sess.TaskID, &sess.ConversationID,
		&sess.ParentSessionID, &sess.ParentRunID, &sess.CurrentRunID,
		&sess.Status, &sess.Metadata, &sess.CreatedAt, &sess.UpdatedAt,
	)
	return sess, err
}
