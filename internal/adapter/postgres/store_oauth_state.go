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

// ConsumeOAuthState deletes an unexpired state and returns it (single use,
// atomic against a concurrent callback with the same state).
//
// INTENTIONALLY CROSS-TENANT: the OAuth callback is a redirect from the
// provider without a CodeForge session, so the request carries no tenant.
// The state is a 256-bit random secret handed only to the user who started
// the flow; the returned row's tenant_id is the tenant the caller acts in.
func (s *Store) ConsumeOAuthState(ctx context.Context, stateToken string) (*vcsaccount.OAuthState, error) {
	var st vcsaccount.OAuthState
	err := s.pool.QueryRow(ctx,
		`DELETE FROM oauth_states
		 WHERE state = $1 AND expires_at > now()
		 RETURNING state, provider, tenant_id, expires_at, created_at`,
		stateToken,
	).Scan(&st.State, &st.Provider, &st.TenantID, &st.ExpiresAt, &st.CreatedAt)
	if err != nil {
		return nil, notFoundWrap(err, "consume oauth state")
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

// deleteExpiredOAuthStatesSQL deletes the states of abandoned OAuth flows.
const deleteExpiredOAuthStatesSQL = `DELETE FROM oauth_states WHERE expires_at <= now()`

// DeleteExpiredOAuthStates deletes the states of abandoned OAuth flows. The
// retention job runs the same statement as a system step of its sweep.
//
// INTENTIONALLY CROSS-TENANT: a system job without a tenant; it deletes
// only rows past their own expiry, which ConsumeOAuthState refuses anyway
// (each row is keyed by its secret state, no tenant data is read).
func (s *Store) DeleteExpiredOAuthStates(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx, deleteExpiredOAuthStatesSQL)
	if err != nil {
		return 0, fmt.Errorf("delete expired oauth states: %w", err)
	}
	return tag.RowsAffected(), nil
}
