package service

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/Strob0t/CodeForge/internal/crypto"
	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/user"
)

// setupGuard holds the one-time token that the first-admin setup of a tenant
// without users requires (KI-119). Only its SHA-256 is kept, so comparing it
// takes the same time whatever the length of the token presented.
type setupGuard struct {
	mu       sync.Mutex
	tenantID string
	hash     [sha256.Size]byte
	armed    bool
}

func (g *setupGuard) arm(tenantID, token string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.tenantID, g.hash, g.armed = tenantID, sha256.Sum256([]byte(token)), true
}

func (g *setupGuard) disarm() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.tenantID, g.hash, g.armed = "", [sha256.Size]byte{}, false
}

func (g *setupGuard) matches(tenantID, token string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.armed || token == "" || tenantID != g.tenantID {
		return false
	}
	given := sha256.Sum256([]byte(token))
	return subtle.ConstantTimeCompare(given[:], g.hash[:]) == 1
}

// PrepareSetupToken arms the one-time setup token on a start without users in
// the tenant: it creates a random token, writes it to auth.setup_token_file
// (mode 0600; skipped when the key is empty or the file cannot be written) and
// logs it once. Each start replaces the token of the previous one. With users
// in the tenant it arms nothing and removes a token file left behind. It
// returns the token, or "" when none is needed.
func (s *AuthService) PrepareSetupToken(ctx context.Context, tenantID string) (string, error) {
	users, err := s.store.ListUsers(ctx, tenantID)
	if err != nil {
		return "", fmt.Errorf("list users: %w", err)
	}
	if len(users) > 0 {
		s.setup.disarm()
		s.removeSetupTokenFile()
		return "", nil
	}

	token, err := crypto.GenerateRandomToken()
	if err != nil {
		return "", fmt.Errorf("generate setup token: %w", err)
	}
	file := s.cfg.SetupTokenFile
	if file != "" {
		if err := writeSecretFile(file, token); err != nil {
			// The log line below still carries the token.
			slog.Warn("failed to write the setup token file", "file", file, "error", err)
			file = ""
		}
	}
	s.setup.arm(tenantID, token)
	slog.Info("SETUP TOKEN: no users yet; open /setup and enter this one-time token to create the first admin",
		"setup_token", token, "file", file)
	return token, nil
}

// CompleteSetup creates the first admin of a tenant without users when token
// is the armed setup token (user.ErrInvalidSetupToken otherwise; the request
// is validated first). The token is used up by a successful setup and its file
// removed. Once the tenant has users it reports domain.ErrConflict.
func (s *AuthService) CompleteSetup(ctx context.Context, req *user.CreateRequest, token string) (*user.User, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %w", domain.ErrValidation, err)
	}
	if !s.setup.matches(req.TenantID, token) {
		status, err := s.GetSetupStatus(ctx, req.TenantID)
		if err == nil && !status.NeedsSetup {
			return nil, fmt.Errorf("setup already done: %w", domain.ErrConflict)
		}
		return nil, user.ErrInvalidSetupToken
	}
	u, err := s.RegisterFirstUser(ctx, req)
	if err != nil {
		return nil, err
	}
	s.setup.disarm()
	s.removeSetupTokenFile()
	slog.Info("first admin created with the setup token; the token is used up", "user_id", u.ID)
	return u, nil
}

func (s *AuthService) removeSetupTokenFile() {
	if s.cfg.SetupTokenFile == "" {
		return
	}
	if err := os.Remove(s.cfg.SetupTokenFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		slog.Warn("failed to remove the setup token file", "file", s.cfg.SetupTokenFile, "error", err)
	}
}

// writeSecretFile writes secret to a new file with mode 0600. An existing file
// is removed first: os.WriteFile would keep its mode, and O_EXCL refuses a
// symlink planted in its place.
func writeSecretFile(path, secret string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove old file: %w", err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // path from trusted config
	if err != nil {
		return err
	}
	if _, err := f.WriteString(secret + "\n"); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}
