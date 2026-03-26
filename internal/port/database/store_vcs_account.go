package database

import (
	"context"

	"github.com/Strob0t/CodeForge/internal/domain/vcsaccount"
)

// VCSAccountStore defines database operations for VCS accounts and OAuth state.
type VCSAccountStore interface {
	// VCS Accounts
	ListVCSAccounts(ctx context.Context) ([]vcsaccount.VCSAccount, error)
	GetVCSAccount(ctx context.Context, id string) (*vcsaccount.VCSAccount, error)
	CreateVCSAccount(ctx context.Context, a *vcsaccount.VCSAccount) (*vcsaccount.VCSAccount, error)
	DeleteVCSAccount(ctx context.Context, id string) error

	// OAuth State (CSRF protection for OAuth flows)
	CreateOAuthState(ctx context.Context, state *vcsaccount.OAuthState) error
	GetOAuthState(ctx context.Context, stateToken string) (*vcsaccount.OAuthState, error)
	DeleteOAuthState(ctx context.Context, stateToken string) error
	DeleteExpiredOAuthStates(ctx context.Context) (int64, error)
}
