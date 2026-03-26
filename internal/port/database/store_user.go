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
}
