package postgres

import (
	"context"
	"fmt"

	"github.com/Strob0t/CodeForge/internal/domain/autoagent"
)

// UpsertAutoAgent creates or updates an auto-agent record for a project.
func (s *Store) UpsertAutoAgent(ctx context.Context, aa *autoagent.AutoAgent) error {
	const q = `
		INSERT INTO auto_agents (project_id, status, current_feature_id, conversation_id,
			features_total, features_complete, features_failed, total_cost_usd, error, started_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW())
		ON CONFLICT (project_id) DO UPDATE SET
			status = EXCLUDED.status,
			current_feature_id = EXCLUDED.current_feature_id,
			conversation_id = EXCLUDED.conversation_id,
			features_total = EXCLUDED.features_total,
			features_complete = EXCLUDED.features_complete,
			features_failed = EXCLUDED.features_failed,
			total_cost_usd = EXCLUDED.total_cost_usd,
			error = EXCLUDED.error,
			started_at = EXCLUDED.started_at,
			updated_at = NOW()
		RETURNING id, updated_at`

	return s.pool.QueryRow(ctx, q,
		aa.ProjectID, string(aa.Status), aa.CurrentFeatureID, aa.ConversationID,
		aa.FeaturesTotal, aa.FeaturesComplete, aa.FeaturesFailed, aa.TotalCostUSD,
		aa.Error, aa.StartedAt,
	).Scan(&aa.ID, &aa.UpdatedAt)
}

// GetAutoAgent retrieves the auto-agent state for a project.
func (s *Store) GetAutoAgent(ctx context.Context, projectID string) (*autoagent.AutoAgent, error) {
	const q = `
		SELECT id, project_id, status, current_feature_id, conversation_id,
			features_total, features_complete, features_failed, total_cost_usd, error,
			started_at, updated_at
		FROM auto_agents
		WHERE project_id = $1`

	var aa autoagent.AutoAgent
	err := s.pool.QueryRow(ctx, q, projectID).Scan(
		&aa.ID, &aa.ProjectID, &aa.Status, &aa.CurrentFeatureID, &aa.ConversationID,
		&aa.FeaturesTotal, &aa.FeaturesComplete, &aa.FeaturesFailed, &aa.TotalCostUSD,
		&aa.Error, &aa.StartedAt, &aa.UpdatedAt,
	)
	if err != nil {
		return nil, notFoundWrap(err, "get auto-agent for project %s", projectID)
	}
	return &aa, nil
}

// UpdateAutoAgentStatus updates the status and error fields of an auto-agent.
func (s *Store) UpdateAutoAgentStatus(ctx context.Context, projectID string, status autoagent.Status, errMsg string) error {
	const q = `
		UPDATE auto_agents
		SET status = $2, error = $3, updated_at = NOW()
		WHERE project_id = $1`

	tag, err := s.pool.Exec(ctx, q, projectID, string(status), errMsg)
	return execExpectOne(tag, err, "update auto-agent status for project %s", projectID)
}

// UpdateAutoAgentProgress updates the progress counters for an auto-agent.
func (s *Store) UpdateAutoAgentProgress(ctx context.Context, aa *autoagent.AutoAgent) error {
	const q = `
		UPDATE auto_agents
		SET current_feature_id = $2, conversation_id = $3,
			features_complete = $4, features_failed = $5,
			total_cost_usd = $6, updated_at = NOW()
		WHERE project_id = $1`

	tag, err := s.pool.Exec(ctx, q,
		aa.ProjectID, aa.CurrentFeatureID, aa.ConversationID,
		aa.FeaturesComplete, aa.FeaturesFailed, aa.TotalCostUSD,
	)
	return execExpectOne(tag, err, "update auto-agent progress for project %s", aa.ProjectID)
}

// DeleteAutoAgent removes the auto-agent record for a project.
func (s *Store) DeleteAutoAgent(ctx context.Context, projectID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM auto_agents WHERE project_id = $1`, projectID)
	if err != nil {
		return fmt.Errorf("delete auto-agent for project %s: %w", projectID, err)
	}
	return nil
}
