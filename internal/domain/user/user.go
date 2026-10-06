// Package user defines the user domain model for authentication and authorization.
package user

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"time"
	"unicode"

	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// Role represents the authorization level of a user.
type Role string

const (
	RoleAdmin  Role = "admin"
	RoleEditor Role = "editor"
	RoleViewer Role = "viewer"
)

// ValidRoles is the set of all valid user roles.
var ValidRoles = map[Role]bool{
	RoleAdmin:  true,
	RoleEditor: true,
	RoleViewer: true,
}

// MaxFailedAttempts is the number of consecutive failed login attempts
// before an account is temporarily locked.
const MaxFailedAttempts = 5

// LockoutDuration is how long an account stays locked after exceeding
// MaxFailedAttempts.
const LockoutDuration = 15 * time.Minute

// The synthetic identities without a row in users: every request acts as
// AuthDisabledUserID while authentication is disabled, the internal service
// key (CODEFORGE_INTERNAL_KEY) acts as InternalServiceUserID, and an audit
// entry whose request resolved no user (a failed login, a password reset
// request) names AnonymousActorUserID as its actor. They are distinct, so
// an audit trail written with authentication disabled still tells the
// operator's actions from failed logins.
const (
	AuthDisabledUserID    = "00000000-0000-0000-0000-000000000000"
	InternalServiceUserID = "00000000-0000-0000-0000-000000000001"
	AnonymousActorUserID  = "00000000-0000-0000-0000-000000000002"
)

// IsAccountless reports whether id is one of the synthetic identities that
// have no users row; any other user ID names an account, which may have been
// deleted or erased since its access token was issued.
func IsAccountless(id string) bool {
	return id == AuthDisabledUserID || id == InternalServiceUserID || id == AnonymousActorUserID
}

// ErrAccountGone reports that a request's user has no account (any more):
// the row was deleted or erased while an access token issued for it is still
// valid. Nothing is recorded under the user's ID or name (GDPR erasure finds
// a user's records by ID).
var ErrAccountGone = errors.New("the user account does not exist")

// ErrInvalidSetupToken reports a first-admin setup without the one-time
// setup token the Core created on its first start (KI-119).
var ErrInvalidSetupToken = errors.New("a valid setup token is required")

// User represents a registered user within a tenant.
type User struct {
	ID                 string    `json:"id"`
	Email              string    `json:"email"`
	Name               string    `json:"name"`
	PasswordHash       string    `json:"-"` // never serialized
	Role               Role      `json:"role"`
	TenantID           string    `json:"tenant_id"`
	Enabled            bool      `json:"enabled"`
	MustChangePassword bool      `json:"must_change_password"`
	FailedAttempts     int       `json:"-"` // consecutive failed login attempts
	LockedUntil        time.Time `json:"-"` // account locked until this time
	TokenEpoch         int64     `json:"-"` // raised to invalidate the user's access tokens (KI-143)
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// IsPlatformAdmin reports whether u administers the platform: an admin of
// the default (bootstrap) tenant. Platform admins manage what all tenants
// share, such as the LiteLLM models and the provider credentials.
func (u *User) IsPlatformAdmin() bool {
	return u.Role == RoleAdmin && u.TenantID == tenantctx.DefaultTenantID
}

// userFields has the fields of User without its methods, so that
// MarshalJSON does not call itself.
type userFields User

// MarshalJSON adds the derived is_platform_admin flag, which the frontend
// uses to show platform-wide actions only to those who may use them.
//
//nolint:gocritic // hugeParam: a value receiver also marshals User fields held by value (LoginResponse.User)
func (u User) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		userFields
		IsPlatformAdmin bool `json:"is_platform_admin"`
	}{userFields(u), u.IsPlatformAdmin()})
}

// IsLocked returns true if the account is currently locked due to
// too many failed login attempts.
func (u *User) IsLocked() bool {
	return !u.LockedUntil.IsZero() && time.Now().Before(u.LockedUntil)
}

// CreateRequest is the input for registering a new user.
type CreateRequest struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Password string `json:"password"` //nolint:gosec // request field, not a hardcoded secret
	Role     Role   `json:"role"`
	TenantID string `json:"tenant_id"`
}

// Validate checks that the CreateRequest has all required fields.
func (r *CreateRequest) Validate() error {
	if r.Email == "" {
		return errors.New("email is required")
	}
	if _, err := mail.ParseAddress(r.Email); err != nil {
		return errors.New("invalid email format")
	}
	if r.Name == "" {
		return errors.New("name is required")
	}
	if r.Password == "" {
		return errors.New("password is required")
	}
	if err := ValidatePasswordComplexity(r.Password); err != nil {
		return fmt.Errorf("password complexity: %w", err)
	}
	if !ValidRoles[r.Role] {
		return errors.New("invalid role: must be admin, editor, or viewer")
	}
	return nil
}

// UpdateRequest is the input for updating an existing user.
type UpdateRequest struct {
	Name    string `json:"name,omitempty"`
	Role    Role   `json:"role,omitempty"`
	Enabled *bool  `json:"enabled,omitempty"`
}

// LoginRequest is the input for user authentication.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"` //nolint:gosec // request field, not a hardcoded secret
}

// Validate checks that the LoginRequest has all required fields.
func (r *LoginRequest) Validate() error {
	if r.Email == "" {
		return errors.New("email is required")
	}
	if r.Password == "" {
		return errors.New("password is required")
	}
	return nil
}

// LoginResponse is returned after successful authentication.
type LoginResponse struct {
	AccessToken string `json:"access_token"` //nolint:gosec // response field, not a hardcoded secret
	ExpiresIn   int    `json:"expires_in"`   // seconds until access token expires
	User        User   `json:"user"`
}

// TokenClaims contains the JWT payload fields.
type TokenClaims struct {
	JTI                string `json:"jti,omitempty"`
	UserID             string `json:"sub"`
	Email              string `json:"email"`
	Name               string `json:"name"`
	Role               Role   `json:"role"`
	TenantID           string `json:"tid"`
	Audience           string `json:"aud,omitempty"`
	Issuer             string `json:"iss,omitempty"`
	IssuedAt           int64  `json:"iat"`
	Expiry             int64  `json:"exp"`
	MustChangePassword bool   `json:"mcp,omitempty"`
	// TokenEpoch is the user's token epoch when the token was issued; a token
	// whose epoch is not the user's current one is refused, and so is a token
	// without it (issued before KI-143).
	TokenEpoch *int64 `json:"epoch,omitempty"`
}

// ChangePasswordRequest is the input for changing a user's password.
type ChangePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

// Validate checks that the ChangePasswordRequest has all required fields.
func (r *ChangePasswordRequest) Validate() error {
	if r.OldPassword == "" {
		return errors.New("old password is required")
	}
	if r.NewPassword == "" {
		return errors.New("new password is required")
	}
	return ValidatePasswordComplexity(r.NewPassword)
}

// ValidatePasswordComplexity checks that a password meets minimum complexity requirements:
// at least 10 characters, contains uppercase, lowercase, and a digit.
func ValidatePasswordComplexity(password string) error {
	if len(password) < 10 {
		return errors.New("password must be at least 10 characters")
	}
	var hasUpper, hasLower, hasDigit bool
	for _, r := range password {
		switch {
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsDigit(r):
			hasDigit = true
		}
	}
	if !hasUpper {
		return errors.New("password must contain at least one uppercase letter")
	}
	if !hasLower {
		return errors.New("password must contain at least one lowercase letter")
	}
	if !hasDigit {
		return errors.New("password must contain at least one digit")
	}
	return nil
}

// RefreshToken represents a stored refresh token.
type RefreshToken struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	TokenHash string    `json:"-"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

// PasswordResetToken represents a stored password reset token.
type PasswordResetToken struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	TenantID  string    `json:"tenant_id"`
	TokenHash string    `json:"-"`
	ExpiresAt time.Time `json:"expires_at"`
	Used      bool      `json:"used"`
	CreatedAt time.Time `json:"created_at"`
}
