package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Strob0t/CodeForge/internal/domain/prompt"
)

// InsertPromptScore inserts a single prompt score signal into the prompt_scores table.
func (s *Store) InsertPromptScore(ctx context.Context, score *prompt.PromptScore) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO prompt_scores
		   (tenant_id, prompt_fingerprint, mode_id, model_family, signal_type, score, run_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		score.TenantID,
		score.PromptFingerprint,
		score.ModeID,
		score.ModelFamily,
		string(score.SignalType),
		score.Score,
		nullIfEmpty(score.RunID),
	)
	if err != nil {
		return fmt.Errorf("insert prompt score: %w", err)
	}
	return nil
}

// GetScoresByFingerprint returns all prompt scores for a given fingerprint.
func (s *Store) GetScoresByFingerprint(ctx context.Context, fingerprint string) ([]prompt.PromptScore, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, tenant_id, prompt_fingerprint, mode_id, model_family,
		        signal_type, score, COALESCE(run_id::text, ''), created_at
		 FROM prompt_scores
		 WHERE prompt_fingerprint = $1
		 ORDER BY created_at DESC`, fingerprint)
	if err != nil {
		return nil, fmt.Errorf("get scores by fingerprint: %w", err)
	}
	return scanRows(rows, func(r pgx.Rows) (prompt.PromptScore, error) {
		var sc prompt.PromptScore
		err := r.Scan(
			&sc.ID, &sc.TenantID, &sc.PromptFingerprint, &sc.ModeID, &sc.ModelFamily,
			&sc.SignalType, &sc.Score, &sc.RunID, &sc.CreatedAt,
		)
		return sc, err
	})
}

// GetAggregatedScores returns per-fingerprint average scores grouped by signal type,
// filtered by tenant, mode, and model family.
func (s *Store) GetAggregatedScores(ctx context.Context, tenantID, modeID, modelFamily string) (map[string]map[prompt.SignalType]float64, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT prompt_fingerprint, signal_type, AVG(score) as avg_score
		 FROM prompt_scores
		 WHERE tenant_id = $1 AND mode_id = $2 AND model_family = $3
		 GROUP BY prompt_fingerprint, signal_type
		 ORDER BY prompt_fingerprint, signal_type`,
		tenantID, modeID, modelFamily)
	if err != nil {
		return nil, fmt.Errorf("get aggregated scores: %w", err)
	}
	defer rows.Close()

	result := make(map[string]map[prompt.SignalType]float64)
	for rows.Next() {
		var fingerprint string
		var signalType prompt.SignalType
		var avgScore float64
		if err := rows.Scan(&fingerprint, &signalType, &avgScore); err != nil {
			return nil, fmt.Errorf("scan aggregated score: %w", err)
		}
		if _, ok := result[fingerprint]; !ok {
			result[fingerprint] = make(map[prompt.SignalType]float64)
		}
		result[fingerprint][signalType] = avgScore
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate aggregated scores: %w", err)
	}
	return result, nil
}
