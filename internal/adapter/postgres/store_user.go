package postgres

import (
	"context"
	"errors"
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

func (s *Store) GetUser(ctx context.Context, id string) (*user.User, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, email, name, password_hash, role, tenant_id, enabled, must_change_password, failed_attempts, locked_until, created_at, updated_at
		FROM users WHERE id = $1`, id)

	var u user.User
	err := row.Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.Role, &u.TenantID, &u.Enabled, &u.MustChangePassword, &u.FailedAttempts, &u.LockedUntil, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("get user %s: %w", id, domain.ErrNotFound)
		}
		return nil, fmt.Errorf("get user: %w", err)
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
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("get user by email %s: %w", email, domain.ErrNotFound)
		}
		return nil, fmt.Errorf("get user by email: %w", err)
	}
	return &u, nil
}

func (s *Store) ListUsers(ctx context.Context, tenantID string) ([]user.User, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, email, name, password_hash, role, tenant_id, enabled, must_change_password, failed_attempts, locked_until, created_at, updated_at
		FROM users WHERE tenant_id = $1 ORDER BY created_at`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	var users []user.User
	for rows.Next() {
		var u user.User
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.Role, &u.TenantID, &u.Enabled, &u.MustChangePassword, &u.FailedAttempts, &u.LockedUntil, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (s *Store) UpdateUser(ctx context.Context, u *user.User) error {
	u.UpdatedAt = time.Now().UTC()
	tag, err := s.pool.Exec(ctx, `
		UPDATE users SET name = $2, role = $3, enabled = $4, must_change_password = $5, failed_attempts = $6, locked_until = $7, updated_at = $8, password_hash = $9
		WHERE id = $1`,
		u.ID, u.Name, u.Role, u.Enabled, u.MustChangePassword, u.FailedAttempts, u.LockedUntil, u.UpdatedAt, u.PasswordHash,
	)
	if err != nil {
		return fmt.Errorf("update user: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("update user %s: %w", u.ID, domain.ErrNotFound)
	}
	return nil
}

func (s *Store) DeleteUser(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("delete user %s: %w", id, domain.ErrNotFound)
	}
	return nil
}

// --- Password Reset Tokens ---

func (s *Store) CreatePasswordResetToken(ctx context.Context, token *user.PasswordResetToken) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO password_reset_tokens (id, user_id, token_hash, expires_at, used, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		token.ID, token.UserID, token.TokenHash, token.ExpiresAt, token.Used, token.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("create password reset token: %w", err)
	}
	return nil
}

func (s *Store) GetPasswordResetTokenByHash(ctx context.Context, tokenHash string) (*user.PasswordResetToken, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, user_id, token_hash, expires_at, used, created_at
		FROM password_reset_tokens WHERE token_hash = $1`, tokenHash)

	var t user.PasswordResetToken
	err := row.Scan(&t.ID, &t.UserID, &t.TokenHash, &t.ExpiresAt, &t.Used, &t.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("get password reset token: %w", domain.ErrNotFound)
		}
		return nil, fmt.Errorf("get password reset token: %w", err)
	}
	return &t, nil
}

func (s *Store) MarkPasswordResetTokenUsed(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE password_reset_tokens SET used = true WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("mark password reset token used: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("mark password reset token %s: %w", id, domain.ErrNotFound)
	}
	return nil
}

func (s *Store) DeleteExpiredPasswordResetTokens(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM password_reset_tokens WHERE expires_at < now() OR used = true`)
	if err != nil {
		return 0, fmt.Errorf("delete expired password reset tokens: %w", err)
	}
	return tag.RowsAffected(), nil
}
