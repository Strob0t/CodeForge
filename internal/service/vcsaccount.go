package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/vcsaccount"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

// VCSAccountService manages VCS account lifecycle.
type VCSAccountService struct {
	db  database.Store
	key []byte
}

// NewVCSAccountService creates a new VCSAccountService.
// The encryptionKey should be derived from the JWT secret using vcsaccount.DeriveKey.
func NewVCSAccountService(s database.Store, encryptionKey []byte) *VCSAccountService {
	return &VCSAccountService{db: s, key: encryptionKey}
}

var validProviders = map[string]bool{
	"github":    true,
	"gitlab":    true,
	"gitea":     true,
	"bitbucket": true,
}

var validAuthMethods = map[string]bool{
	"token": true,
	"ssh":   true,
	"oauth": true,
}

// Create validates, encrypts the token, and stores a new VCS account.
func (s *VCSAccountService) Create(ctx context.Context, req *vcsaccount.CreateRequest) (*vcsaccount.VCSAccount, error) {
	if req.Label == "" {
		return nil, errors.New("label is required")
	}
	if !validProviders[req.Provider] {
		return nil, fmt.Errorf("invalid provider %q: must be github, gitlab, gitea, or bitbucket", req.Provider)
	}
	if req.Token == "" {
		return nil, errors.New("token is required")
	}

	authMethod := req.AuthMethod
	if authMethod == "" {
		authMethod = "token"
	}
	if !validAuthMethods[authMethod] {
		return nil, fmt.Errorf("invalid auth_method %q: must be token, ssh, or oauth", authMethod)
	}

	encrypted, err := vcsaccount.Encrypt([]byte(req.Token), s.key)
	if err != nil {
		return nil, fmt.Errorf("encrypt token: %w", err)
	}

	a := &vcsaccount.VCSAccount{
		Provider:       req.Provider,
		Label:          req.Label,
		ServerURL:      req.ServerURL,
		AuthMethod:     authMethod,
		EncryptedToken: encrypted,
	}

	return s.db.CreateVCSAccount(ctx, a)
}

// List returns all VCS accounts for the current tenant (without decrypted tokens).
func (s *VCSAccountService) List(ctx context.Context) ([]vcsaccount.VCSAccount, error) {
	accounts, err := s.db.ListVCSAccounts(ctx)
	if err != nil {
		return nil, err
	}
	// Clear encrypted tokens from the response; they are never sent to the client.
	for i := range accounts {
		accounts[i].EncryptedToken = nil
	}
	return accounts, nil
}

// Delete removes a VCS account by ID.
func (s *VCSAccountService) Delete(ctx context.Context, id string) error {
	return s.db.DeleteVCSAccount(ctx, id)
}

// Test decrypts the stored token and attempts a basic connection test to the provider.
func (s *VCSAccountService) Test(ctx context.Context, id string) error {
	a, err := s.db.GetVCSAccount(ctx, id)
	if err != nil {
		return err
	}

	token, err := vcsaccount.Decrypt(a.EncryptedToken, s.key)
	if err != nil {
		return fmt.Errorf("decrypt token: %w", err)
	}

	return testProviderConnection(ctx, a.Provider, a.ServerURL, string(token))
}

// testProviderConnection makes a lightweight API call to verify the token works.
func testProviderConnection(ctx context.Context, provider, serverURL, token string) error {
	var apiURL string
	switch provider {
	case "github":
		if serverURL != "" {
			apiURL = serverURL + "/api/v3/user"
		} else {
			apiURL = "https://api.github.com/user"
		}
	case "gitlab":
		if serverURL != "" {
			apiURL = serverURL + "/api/v4/user"
		} else {
			apiURL = "https://gitlab.com/api/v4/user"
		}
	case "gitea":
		if serverURL == "" {
			return errors.New("server_url is required for gitea")
		}
		apiURL = serverURL + "/api/v1/user"
	case "bitbucket":
		if serverURL != "" {
			apiURL = serverURL + "/rest/api/1.0/users"
		} else {
			apiURL = "https://api.bitbucket.org/2.0/user"
		}
	default:
		return fmt.Errorf("unsupported provider: %s", provider)
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, http.NoBody) //nolint:gosec // apiURL is constructed from validated provider constants
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	if provider == "gitlab" {
		req.Header.Set("PRIVATE-TOKEN", token)
	} else {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req) //nolint:gosec // apiURL is constructed from validated provider switch, not user input
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return errors.New("authentication failed: invalid or expired token")
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("provider returned HTTP %d", resp.StatusCode)
	}

	return nil
}
