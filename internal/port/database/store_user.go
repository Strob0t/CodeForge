package database

import (
	"context"

	"github.com/Strob0t/CodeForge/internal/domain/user"
)

// UserStore defines database operations for user management.
type UserStore interface {
	CreateUser(ctx context.Context, u *user.User) error
	GetUser(ctx context.Context, id string) (*user.User, error)
	GetUserByEmail(ctx context.Context, email, tenantID string) (*user.User, error)
	ListUsers(ctx context.Context, tenantID string) ([]user.User, error)
	UpdateUser(ctx context.Context, u *user.User) error
	DeleteUser(ctx context.Context, id string) error

	// CreateFirstUser atomically creates the first admin user if none exist.
	CreateFirstUser(ctx context.Context, u *user.User) error

	// GetUserTokenEpoch returns the token epoch of the user in the tenant
	// (domain.ErrNotFound when the user does not exist); access token
	// validation compares it with the token's epoch (KI-143).
	GetUserTokenEpoch(ctx context.Context, userID, tenantID string) (int64, error)
	// RaiseUserTokenEpoch increments the user's token epoch, which
	// invalidates every access token issued to the user before.
	RaiseUserTokenEpoch(ctx context.Context, userID string) error
}
