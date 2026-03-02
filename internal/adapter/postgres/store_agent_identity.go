package postgres

import (
	"context"
	"fmt"
	"time"

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
		WHERE id = $1`

	tag, err := s.pool.Exec(ctx, q, id, costDelta, successVal, now)
	return execExpectOne(tag, err, "increment agent stats for %s", id)
}

// UpdateAgentState replaces the agent's key-value state map.
func (s *Store) UpdateAgentState(ctx context.Context, id string, state map[string]string) error {
	now := time.Now().UTC()
	const q = `
		UPDATE agents
		SET state = $2, updated_at = $3
		WHERE id = $1`

	tag, err := s.pool.Exec(ctx, q, id, state, now)
	return execExpectOne(tag, err, "update agent state for %s", id)
}

// SendAgentMessage inserts a new inbox message.
func (s *Store) SendAgentMessage(ctx context.Context, msg *agent.InboxMessage) error {
	msg.CreatedAt = time.Now().UTC()
	const q = `
		INSERT INTO agent_inbox (agent_id, from_agent, content, priority, created_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id`

	return s.pool.QueryRow(ctx, q,
		msg.AgentID, msg.FromAgent, msg.Content, msg.Priority, msg.CreatedAt,
	).Scan(&msg.ID)
}

// ListAgentInbox returns inbox messages for an agent, optionally filtered to unread only.
func (s *Store) ListAgentInbox(ctx context.Context, agentID string, unreadOnly bool) ([]agent.InboxMessage, error) {
	var q string
	var args []interface{}

	if unreadOnly {
		q = `
			SELECT id, agent_id, from_agent, content, priority, read, created_at
			FROM agent_inbox
			WHERE agent_id = $1 AND read = false
			ORDER BY priority DESC, created_at ASC`
		args = []interface{}{agentID}
	} else {
		q = `
			SELECT id, agent_id, from_agent, content, priority, read, created_at
			FROM agent_inbox
			WHERE agent_id = $1
			ORDER BY priority DESC, created_at ASC`
		args = []interface{}{agentID}
	}

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list agent inbox: %w", err)
	}
	defer rows.Close()

	var result []agent.InboxMessage
	for rows.Next() {
		var msg agent.InboxMessage
		if err := rows.Scan(
			&msg.ID, &msg.AgentID, &msg.FromAgent, &msg.Content,
			&msg.Priority, &msg.Read, &msg.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan agent inbox message: %w", err)
		}
		result = append(result, msg)
	}
	return result, rows.Err()
}

// MarkInboxRead marks a single inbox message as read.
func (s *Store) MarkInboxRead(ctx context.Context, messageID string) error {
	const q = `UPDATE agent_inbox SET read = true WHERE id = $1`
	tag, err := s.pool.Exec(ctx, q, messageID)
	return execExpectOne(tag, err, "mark inbox message %s as read", messageID)
}
