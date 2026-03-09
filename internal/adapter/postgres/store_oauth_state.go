package postgres

import (
	"context"
	"fmt"

	"github.com/Strob0t/CodeForge/internal/domain/vcsaccount"
)

func (s *Store) CreateOAuthState(ctx context.Context, state *vcsaccount.OAuthState) error {
	tid := tenantFromCtx(ctx)
	_, err := s.pool.Exec(ctx,
		`INSERT INTO oauth_states (state, provider, tenant_id, expires_at, created_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		state.State, state.Provider, tid, state.ExpiresAt, state.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("create oauth state: %w", err)
	}
	return nil
}

func (s *Store) GetOAuthState(ctx context.Context, stateToken string) (*vcsaccount.OAuthState, error) {
	tid := tenantFromCtx(ctx)
	var st vcsaccount.OAuthState
	err := s.pool.QueryRow(ctx,
		`SELECT state, provider, tenant_id, expires_at, created_at
		 FROM oauth_states
		 WHERE state = $1 AND tenant_id = $2 AND expires_at > now()`,
		stateToken, tid,
	).Scan(&st.State, &st.Provider, &st.TenantID, &st.ExpiresAt, &st.CreatedAt)
	if err != nil {
		return nil, notFoundWrap(err, "get oauth state %s", stateToken)
	}
	return &st, nil
}

func (s *Store) DeleteOAuthState(ctx context.Context, stateToken string) error {
	tid := tenantFromCtx(ctx)
	_, err := s.pool.Exec(ctx,
		`DELETE FROM oauth_states WHERE state = $1 AND tenant_id = $2`,
		stateToken, tid,
	)
	if err != nil {
		return fmt.Errorf("delete oauth state: %w", err)
	}
	return nil
}

func (s *Store) DeleteExpiredOAuthStates(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM oauth_states WHERE expires_at <= now()`,
	)
	if err != nil {
		return 0, fmt.Errorf("delete expired oauth states: %w", err)
	}
	return tag.RowsAffected(), nil
}
