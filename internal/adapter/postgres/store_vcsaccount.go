package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/vcsaccount"
)

// --- VCS Account CRUD ---

func (s *Store) ListVCSAccounts(ctx context.Context) ([]vcsaccount.VCSAccount, error) {
	tid := tenantFromCtx(ctx)
	rows, err := s.pool.Query(ctx,
		`SELECT id, tenant_id, provider, label, server_url, auth_method, encrypted_token, created_at, updated_at
		 FROM vcs_accounts WHERE tenant_id = $1 ORDER BY created_at ASC`, tid)
	if err != nil {
		return nil, fmt.Errorf("list vcs accounts: %w", err)
	}
	defer rows.Close()

	var accounts []vcsaccount.VCSAccount
	for rows.Next() {
		var a vcsaccount.VCSAccount
		if err := rows.Scan(&a.ID, &a.TenantID, &a.Provider, &a.Label, &a.ServerURL,
			&a.AuthMethod, &a.EncryptedToken, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan vcs account: %w", err)
		}
		accounts = append(accounts, a)
	}
	return accounts, rows.Err()
}

func (s *Store) GetVCSAccount(ctx context.Context, id string) (*vcsaccount.VCSAccount, error) {
	var a vcsaccount.VCSAccount
	err := s.pool.QueryRow(ctx,
		`SELECT id, tenant_id, provider, label, server_url, auth_method, encrypted_token, created_at, updated_at
		 FROM vcs_accounts WHERE id = $1`, id,
	).Scan(&a.ID, &a.TenantID, &a.Provider, &a.Label, &a.ServerURL,
		&a.AuthMethod, &a.EncryptedToken, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("get vcs account %s: %w", id, domain.ErrNotFound)
		}
		return nil, fmt.Errorf("get vcs account %s: %w", id, err)
	}
	return &a, nil
}

func (s *Store) CreateVCSAccount(ctx context.Context, a *vcsaccount.VCSAccount) (*vcsaccount.VCSAccount, error) {
	tid := tenantFromCtx(ctx)
	var created vcsaccount.VCSAccount
	err := s.pool.QueryRow(ctx,
		`INSERT INTO vcs_accounts (tenant_id, provider, label, server_url, auth_method, encrypted_token)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, tenant_id, provider, label, server_url, auth_method, encrypted_token, created_at, updated_at`,
		tid, a.Provider, a.Label, a.ServerURL, a.AuthMethod, a.EncryptedToken,
	).Scan(&created.ID, &created.TenantID, &created.Provider, &created.Label, &created.ServerURL,
		&created.AuthMethod, &created.EncryptedToken, &created.CreatedAt, &created.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create vcs account: %w", err)
	}
	return &created, nil
}

func (s *Store) DeleteVCSAccount(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM vcs_accounts WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete vcs account %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("delete vcs account %s: %w", id, domain.ErrNotFound)
	}
	return nil
}
