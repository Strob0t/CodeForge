package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Strob0t/CodeForge/internal/domain/agent"
)

// IncrementAgentStats atomically updates run count, cost, and success rate for an agent.
func (s *Store) IncrementAgentStats(ctx context.Context, id string, costDelta float64, success bool) error {
	now := time.Now().UTC()

	// Compute new success rate inline: rate = (rate*runs + success) / (runs+1)
	successVal := 0
	if success {
		successVal = 1
	}

	const q = `
		UPDATE agents
		SET total_runs   = total_runs + 1,
			total_cost   = total_cost + $2,
			success_rate = (success_rate * total_runs + $3) / (total_runs + 1),
			last_active_at = $4,
			updated_at   = $4
		WHERE id = $1 AND tenant_id = $5`

	tag, err := s.pool.Exec(ctx, q, id, costDelta, successVal, now, tenantFromCtx(ctx))
	return execExpectOne(tag, err, "increment agent stats for %s", id)
}

// UpdateAgentState replaces the agent's key-value state map.
func (s *Store) UpdateAgentState(ctx context.Context, id string, state map[string]string) error {
	now := time.Now().UTC()
	const q = `
		UPDATE agents
		SET state = $2, updated_at = $3
		WHERE id = $1 AND tenant_id = $4`

	tag, err := s.pool.Exec(ctx, q, id, state, now, tenantFromCtx(ctx))
	return execExpectOne(tag, err, "update agent state for %s", id)
}

// SendAgentMessage inserts a new inbox message.
func (s *Store) SendAgentMessage(ctx context.Context, msg *agent.InboxMessage) error {
	msg.CreatedAt = time.Now().UTC()
	const q = `
		INSERT INTO agent_inbox (agent_id, from_agent, content, priority, created_at, tenant_id)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id`

	return s.pool.QueryRow(ctx, q,
		msg.AgentID, msg.FromAgent, msg.Content, msg.Priority, msg.CreatedAt, tenantFromCtx(ctx),
	).Scan(&msg.ID)
}

// ListAgentInbox returns inbox messages for an agent, optionally filtered to unread only.
func (s *Store) ListAgentInbox(ctx context.Context, agentID string, unreadOnly bool) ([]agent.InboxMessage, error) {
	var q string
	var args []any

	tid := tenantFromCtx(ctx)
	if unreadOnly {
		q = `
			SELECT id, agent_id, from_agent, content, priority, read, created_at
			FROM agent_inbox
			WHERE agent_id = $1 AND read = false AND tenant_id = $2
			ORDER BY priority DESC, created_at ASC
			LIMIT $3`
		args = []any{agentID, tid, DefaultListLimit}
	} else {
		q = `
			SELECT id, agent_id, from_agent, content, priority, read, created_at
			FROM agent_inbox
			WHERE agent_id = $1 AND tenant_id = $2
			ORDER BY priority DESC, created_at ASC
			LIMIT $3`
		args = []any{agentID, tid, DefaultListLimit}
	}

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list agent inbox: %w", err)
	}
	return scanRows(rows, func(r pgx.Rows) (agent.InboxMessage, error) {
		var msg agent.InboxMessage
		err := r.Scan(
			&msg.ID, &msg.AgentID, &msg.FromAgent, &msg.Content,
			&msg.Priority, &msg.Read, &msg.CreatedAt,
		)
		return msg, err
	})
}

// MarkInboxRead marks a single inbox message as read.
func (s *Store) MarkInboxRead(ctx context.Context, messageID string) error {
	const q = `UPDATE agent_inbox SET read = true WHERE id = $1 AND tenant_id = $2`
	tag, err := s.pool.Exec(ctx, q, messageID, tenantFromCtx(ctx))
	return execExpectOne(tag, err, "mark inbox message %s as read", messageID)
}

// ClaimHandoff records that the stage of the handoff handoffID is being
// carried out in the caller's tenant (see database.AgentStore).
func (s *Store) ClaimHandoff(ctx context.Context, handoffID, stage string) (bool, error) {
	tag, err := s.pool.Exec(ctx,
		`INSERT INTO handoff_claims (tenant_id, handoff_id, stage) VALUES ($1, $2, $3)
		 ON CONFLICT (tenant_id, handoff_id, stage) DO NOTHING`,
		tenantFromCtx(ctx), handoffID, stage)
	if err != nil {
		return false, fmt.Errorf("claim handoff %s: %w", handoffID, err)
	}
	return tag.RowsAffected() == 1, nil
}

// ReleaseHandoff removes the caller's tenant's claim of a handoff stage.
func (s *Store) ReleaseHandoff(ctx context.Context, handoffID, stage string) error {
	if _, err := s.pool.Exec(ctx,
		`DELETE FROM handoff_claims WHERE tenant_id = $1 AND handoff_id = $2 AND stage = $3`,
		tenantFromCtx(ctx), handoffID, stage); err != nil {
		return fmt.Errorf("release handoff %s: %w", handoffID, err)
	}
	return nil
}
