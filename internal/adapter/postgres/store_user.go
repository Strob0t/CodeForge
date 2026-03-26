package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/user"
)

func (s *Store) CreateUser(ctx context.Context, u *user.User) error {
	now := time.Now().UTC()
	u.CreatedAt = now
	u.UpdatedAt = now

	_, err := s.pool.Exec(ctx, `
		INSERT INTO users (id, email, name, password_hash, role, tenant_id, enabled, must_change_password, failed_attempts, locked_until, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		u.ID, u.Email, u.Name, u.PasswordHash, u.Role, u.TenantID, u.Enabled, u.MustChangePassword, u.FailedAttempts, u.LockedUntil, u.CreatedAt, u.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}

// CreateFirstUser atomically creates a user only if no users exist for the tenant.
// Uses a PostgreSQL advisory lock to prevent concurrent setup race conditions (CWE-367).
// Returns domain.ErrConflict if any user already exists.
func (s *Store) CreateFirstUser(ctx context.Context, u *user.User) error {
	now := time.Now().UTC()
	u.CreatedAt = now
	u.UpdatedAt = now

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback on defer is best-effort

	// Advisory lock scoped to transaction prevents concurrent setup attempts.
	// The lock key 0x436F6465466F7267 is derived from "CodeForg" in ASCII hex.
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(4846791580151137143)"); err != nil {
		return fmt.Errorf("advisory lock: %w", err)
	}

	var count int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM users WHERE tenant_id = $1", u.TenantID).Scan(&count); err != nil {
		return fmt.Errorf("count users: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("create first user: %w", domain.ErrConflict)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO users (id, email, name, password_hash, role, tenant_id, enabled, must_change_password, failed_attempts, locked_until, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		u.ID, u.Email, u.Name, u.PasswordHash, u.Role, u.TenantID, u.Enabled, u.MustChangePassword, u.FailedAttempts, u.LockedUntil, u.CreatedAt, u.UpdatedAt,
	); err != nil {
		return fmt.Errorf("create first user: %w", err)
	}

	return tx.Commit(ctx)
}

// GetUser retrieves a user by ID. This is intentionally cross-tenant because
// it is used during authentication flows (token validation, API key lookup)
// where the caller's tenant context is not yet established.
func (s *Store) GetUser(ctx context.Context, id string) (*user.User, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, email, name, password_hash, role, tenant_id, enabled, must_change_password, failed_attempts, locked_until, created_at, updated_at
		FROM users WHERE id = $1`, id)

	var u user.User
	err := row.Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.Role, &u.TenantID, &u.Enabled, &u.MustChangePassword, &u.FailedAttempts, &u.LockedUntil, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, notFoundWrap(err, "get user %s", id)
	}
	return &u, nil
}

func (s *Store) GetUserByEmail(ctx context.Context, email, tenantID string) (*user.User, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, email, name, password_hash, role, tenant_id, enabled, must_change_password, failed_attempts, locked_until, created_at, updated_at
		FROM users WHERE email = $1 AND tenant_id = $2`, email, tenantID)

	var u user.User
	err := row.Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.Role, &u.TenantID, &u.Enabled, &u.MustChangePassword, &u.FailedAttempts, &u.LockedUntil, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, notFoundWrap(err, "get user by email %s", email)
	}
	return &u, nil
}

func (s *Store) ListUsers(ctx context.Context, tenantID string) ([]user.User, error) {
	// FIX-094: Explicit column list excluding password_hash — listing users
	// should never expose credential data to the API layer.
	rows, err := s.pool.Query(ctx, `
		SELECT id, email, name, role, tenant_id, enabled, must_change_password, failed_attempts, locked_until, created_at, updated_at
		FROM users WHERE tenant_id = $1 ORDER BY created_at`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	return scanRows(rows, func(r pgx.Rows) (user.User, error) {
		var u user.User
		err := r.Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.TenantID, &u.Enabled, &u.MustChangePassword, &u.FailedAttempts, &u.LockedUntil, &u.CreatedAt, &u.UpdatedAt)
		return u, err
	})
}

func (s *Store) UpdateUser(ctx context.Context, u *user.User) error {
	u.UpdatedAt = time.Now().UTC()
	tag, err := s.pool.Exec(ctx, `
		UPDATE users SET name = $2, role = $3, enabled = $4, must_change_password = $5, failed_attempts = $6, locked_until = $7, updated_at = $8, password_hash = $9
		WHERE id = $1 AND tenant_id = $10`,
		u.ID, u.Name, u.Role, u.Enabled, u.MustChangePassword, u.FailedAttempts, u.LockedUntil, u.UpdatedAt, u.PasswordHash, tenantFromCtx(ctx),
	)
	return execExpectOne(tag, err, "update user %s", u.ID)
}

func (s *Store) DeleteUser(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM users WHERE id = $1 AND tenant_id = $2`, id, tenantFromCtx(ctx))
	return execExpectOne(tag, err, "delete user %s", id)
}

// --- Password Reset Tokens ---

func (s *Store) CreatePasswordResetToken(ctx context.Context, token *user.PasswordResetToken) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO password_reset_tokens (id, user_id, tenant_id, token_hash, expires_at, used, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		token.ID, token.UserID, tenantFromCtx(ctx), token.TokenHash, token.ExpiresAt, token.Used, token.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("create password reset token: %w", err)
	}
	return nil
}

func (s *Store) GetPasswordResetTokenByHash(ctx context.Context, tokenHash string) (*user.PasswordResetToken, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, user_id, tenant_id, token_hash, expires_at, used, created_at
		FROM password_reset_tokens WHERE token_hash = $1 AND tenant_id = $2`, tokenHash, tenantFromCtx(ctx))

	var t user.PasswordResetToken
	err := row.Scan(&t.ID, &t.UserID, &t.TenantID, &t.TokenHash, &t.ExpiresAt, &t.Used, &t.CreatedAt)
	if err != nil {
		return nil, notFoundWrap(err, "get password reset token")
	}
	return &t, nil
}

func (s *Store) MarkPasswordResetTokenUsed(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE password_reset_tokens SET used = true WHERE id = $1 AND tenant_id = $2`, id, tenantFromCtx(ctx))
	return execExpectOne(tag, err, "mark password reset token %s", id)
}

func (s *Store) DeleteExpiredPasswordResetTokens(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM password_reset_tokens WHERE (expires_at < now() OR used = true) AND tenant_id = $1`, tenantFromCtx(ctx))
	if err != nil {
		return 0, fmt.Errorf("delete expired password reset tokens: %w", err)
	}
	return tag.RowsAffected(), nil
}
